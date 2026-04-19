package game

import (
	"io"
	"log"
	"math"
	mathrand "math/rand"
	"testing"
	"time"
)

func newClassicTestServer() *Server {
	cfg := Config{
		LobbyID:             "lobby-test",
		RedisAddr:           "localhost:6379",
		TickRate:            60,
		SnapshotRate:        20,
		MaxPlayers:          10,
		WorldWidth:          1200,
		WorldHeight:         800,
		GravityAccel:        2400,
		Drag:                0.98,
		TerminalSpeed:       900,
		NumObjects:          8,
		StartingMass:        10,
		StartingHealth:      100,
		ProjectileSpeed:     1250,
		ProjectileDamage:    28,
		ProjectileRadius:    5,
		ShootCooldown:       250 * time.Millisecond,
		ProjectileTTL:       1200 * time.Millisecond,
		MatchDuration:       time.Minute,
		RespawnDelay:        2 * time.Second,
		BotFillDelay:        time.Hour,
		HealthTickThreshold: 2 * time.Second,
	}
	return NewServer(cfg, log.New(io.Discard, "", 0))
}

func TestNormalizedCurvesAndHealthRescaling(t *testing.T) {
	server := newClassicTestServer()

	if got := server.maxHealthForMass(server.cfg.StartingMass); math.Abs(got-server.cfg.HealthBase) > 0.001 {
		t.Fatalf("maxHealthForMass(startingMass) = %v, want %v", got, server.cfg.HealthBase)
	}
	if got := server.radiusForMass(server.cfg.StartingMass); math.Abs(got-server.cfg.RadiusBase) > 0.001 {
		t.Fatalf("radiusForMass(startingMass) = %v, want %v", got, server.cfg.RadiusBase)
	}

	got := server.rescaleHealthForMassChange(50, 10, 20)
	want := 0.5 * server.maxHealthForMass(20)
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("rescaled health = %v, want %v", got, want)
	}
}

func TestResolveCombatIgnoresInvulnerableProjectileTargets(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()

	owner := &Player{
		ID:     "owner",
		Name:   "Owner",
		Alive:  true,
		Color:  "#fff",
		Mass:   server.cfg.StartingMass,
		Health: server.maxHealthForMass(server.cfg.StartingMass),
	}
	victim := &Player{
		ID:                     "victim",
		Name:                   "Victim",
		Alive:                  true,
		Color:                  "#000",
		Mass:                   server.cfg.StartingMass,
		Health:                 server.maxHealthForMass(server.cfg.StartingMass),
		X:                      300,
		Y:                      300,
		SpawnInvulnerableUntil: now.Add(time.Second),
	}

	server.lobby.Players[owner.ID] = owner
	server.lobby.Players[victim.ID] = victim
	server.lobby.Projectiles = []*Projectile{{
		ID:        "shot-1",
		OwnerID:   owner.ID,
		X:         victim.X,
		Y:         victim.Y,
		Radius:    server.cfg.ProjectileRadius,
		Damage:    server.cfg.ProjectileDamage,
		ExpiresAt: now.Add(time.Second),
	}}

	server.resolveCombatLocked(now)

	if victim.Health != server.maxHealthForMass(victim.Mass) {
		t.Fatalf("victim health changed while invulnerable: got %v", victim.Health)
	}
	if got := len(server.lobby.Projectiles); got != 1 {
		t.Fatalf("projectile count = %d, want 1", got)
	}
}

func TestObjectPickupAllowedDuringInvulnerability(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()

	player := &Player{
		ID:                     "player-1",
		Alive:                  true,
		Mass:                   server.cfg.StartingMass,
		Health:                 server.maxHealthForMass(server.cfg.StartingMass),
		X:                      220,
		Y:                      220,
		SpawnInvulnerableUntil: now.Add(time.Second),
	}
	server.lobby.Players[player.ID] = player
	server.lobby.Objects = []*Collectible{{
		ID:     "obj-1",
		X:      player.X,
		Y:      player.Y,
		Radius: 6,
		Mass:   1.5,
	}}

	server.resolveObjectCollisionsLocked(now)

	if player.Mass <= server.cfg.StartingMass {
		t.Fatalf("player mass did not increase during invulnerable pickup: got %v", player.Mass)
	}
	if got := server.lobby.Objects[0].ID; got == "obj-1" {
		t.Fatalf("object was not respawned after pickup")
	}
	if player.LastPickupFeedbackSeq != 1 {
		t.Fatalf("pickup feedback sequence = %d, want 1", player.LastPickupFeedbackSeq)
	}
}

func TestObjectPickupHealsByMaxHealthGainAndStoresFeedback(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()

	player := &Player{
		ID:     "player-1",
		Alive:  true,
		Mass:   server.cfg.StartingMass,
		Health: 55,
		X:      220,
		Y:      220,
	}
	objectMass := 2.0
	server.lobby.Players[player.ID] = player
	server.lobby.Objects = []*Collectible{{
		ID:     "obj-1",
		X:      player.X,
		Y:      player.Y,
		Radius: 8,
		Mass:   objectMass,
	}}

	oldMaxHealth := server.maxHealthForMass(player.Mass)
	newMaxHealth := server.maxHealthForMass(player.Mass + objectMass)
	wantHealthGain := newMaxHealth - oldMaxHealth

	server.resolveObjectCollisionsLocked(now)

	if math.Abs(player.Mass-(server.cfg.StartingMass+objectMass)) > 0.001 {
		t.Fatalf("player mass = %v, want %v", player.Mass, server.cfg.StartingMass+objectMass)
	}
	if math.Abs(player.Health-(55+wantHealthGain)) > 0.001 {
		t.Fatalf("player health = %v, want %v", player.Health, 55+wantHealthGain)
	}
	if player.LastPickupFeedbackSeq != 1 {
		t.Fatalf("pickup feedback sequence = %d, want 1", player.LastPickupFeedbackSeq)
	}
	if math.Abs(player.LastPickupMassGain-objectMass) > 0.001 {
		t.Fatalf("pickup mass gain = %v, want %v", player.LastPickupMassGain, objectMass)
	}
	if math.Abs(player.LastPickupHealthGain-wantHealthGain) > 0.001 {
		t.Fatalf("pickup health gain = %v, want %v", player.LastPickupHealthGain, wantHealthGain)
	}
}

func TestProjectileKillAppliesMassTransferAndHealAfterTransfer(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()

	owner := &Player{
		ID:     "owner",
		Name:   "Owner",
		Alive:  true,
		Color:  "#fff",
		Mass:   server.cfg.StartingMass,
		Health: 50,
	}
	victim := &Player{
		ID:     "victim",
		Name:   "Victim",
		Alive:  true,
		Color:  "#000",
		Mass:   20,
		Health: 5,
		X:      300,
		Y:      300,
	}

	server.lobby.Players[owner.ID] = owner
	server.lobby.Players[victim.ID] = victim
	server.lobby.Projectiles = []*Projectile{{
		ID:        "shot-1",
		OwnerID:   owner.ID,
		X:         victim.X,
		Y:         victim.Y,
		Radius:    server.cfg.ProjectileRadius,
		Damage:    server.cfg.ProjectileDamage,
		ExpiresAt: now.Add(time.Second),
	}}

	server.resolveCombatLocked(now)

	expectedMass := 20*server.cfg.KillMassTransferPct + server.cfg.StartingMass
	if math.Abs(owner.Mass-expectedMass) > 0.001 {
		t.Fatalf("owner mass = %v, want %v", owner.Mass, expectedMass)
	}

	expectedHealth := server.rescaleHealthForMassChange(50, 10, expectedMass)
	expectedHealth = math.Min(
		server.maxHealthForMass(expectedMass),
		expectedHealth+server.killHealAmount(expectedMass),
	)
	if math.Abs(owner.Health-expectedHealth) > 0.001 {
		t.Fatalf("owner health = %v, want %v", owner.Health, expectedHealth)
	}
	if victim.Alive {
		t.Fatalf("victim should be dead after lethal projectile")
	}
}

func TestCrashDamageUsesOpponentMaxHealth(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()

	left := &Player{
		ID:     "left",
		Name:   "Left",
		Alive:  true,
		Mass:   20,
		Health: server.maxHealthForMass(20),
		X:      300,
		Y:      300,
	}
	right := &Player{
		ID:     "right",
		Name:   "Right",
		Alive:  true,
		Mass:   10,
		Health: server.maxHealthForMass(10),
		X:      300,
		Y:      300,
	}

	server.lobby.Players[left.ID] = left
	server.lobby.Players[right.ID] = right

	server.resolveCombatLocked(now)

	wantLeftHealth := math.Max(0, server.maxHealthForMass(20)-server.crashDamageForPlayer(right))
	wantRightHealth := math.Max(0, server.maxHealthForMass(10)-server.crashDamageForPlayer(left))
	if math.Abs(left.Health-wantLeftHealth) > 0.001 {
		t.Fatalf("left health = %v, want %v", left.Health, wantLeftHealth)
	}
	if math.Abs(right.Health-wantRightHealth) > 0.001 {
		t.Fatalf("right health = %v, want %v", right.Health, wantRightHealth)
	}
}

func TestRespawnUsesRetentionAndInvulnerability(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()

	player := &Player{
		ID:           "player-1",
		Name:         "Pilot",
		Mass:         40,
		Health:       0,
		Alive:        false,
		PreDeathMass: 40,
		RespawnAt:    now.Add(-time.Second),
	}
	server.lobby.Players[player.ID] = player

	server.handleRespawnsLocked(now)

	expectedMass := server.respawnMass(40)
	if !player.Alive {
		t.Fatalf("player did not respawn")
	}
	if math.Abs(player.Mass-expectedMass) > 0.001 {
		t.Fatalf("respawn mass = %v, want %v", player.Mass, expectedMass)
	}
	if math.Abs(player.Health-server.maxHealthForMass(expectedMass)) > 0.001 {
		t.Fatalf("respawn health = %v, want full max health %v", player.Health, server.maxHealthForMass(expectedMass))
	}
	if !player.SpawnInvulnerableUntil.Equal(now.Add(server.cfg.SpawnInvulnerabilityDuration)) {
		t.Fatalf("spawn invulnerability expiry = %v, want %v", player.SpawnInvulnerableUntil, now.Add(server.cfg.SpawnInvulnerabilityDuration))
	}
}

func TestUpsertHumanPlayerInitialSpawnUsesProtectionAndFallbackSeparation(t *testing.T) {
	server := newClassicTestServer()
	server.cfg.SpawnClearanceAttempts = 0
	now := time.Now()

	connection := &ClientConnection{PlayerID: "human-1"}
	player := server.upsertHumanPlayerLocked("human-1", "Pilot", connection, now)

	if !player.SpawnInvulnerableUntil.Equal(now.Add(server.cfg.SpawnInvulnerabilityDuration)) {
		t.Fatalf("spawn invulnerability expiry = %v, want %v", player.SpawnInvulnerableUntil, now.Add(server.cfg.SpawnInvulnerabilityDuration))
	}
	if !player.PendingSpawnSeparation {
		t.Fatalf("pending spawn separation = false, want true when fallback spawn is used")
	}
}

func TestNewBotSpawnUsesProtectionAndFallbackSeparation(t *testing.T) {
	server := newClassicTestServer()
	server.cfg.SpawnClearanceAttempts = 0
	now := time.Now()

	bot := server.newBotLocked(now)

	if !bot.SpawnInvulnerableUntil.Equal(now.Add(server.cfg.SpawnInvulnerabilityDuration)) {
		t.Fatalf("spawn invulnerability expiry = %v, want %v", bot.SpawnInvulnerableUntil, now.Add(server.cfg.SpawnInvulnerabilityDuration))
	}
	if !bot.PendingSpawnSeparation {
		t.Fatalf("pending spawn separation = false, want true when fallback spawn is used")
	}
}

func TestResetMatchSpawnUsesProtectionAndFallbackSeparation(t *testing.T) {
	server := newClassicTestServer()
	server.cfg.SpawnClearanceAttempts = 0
	now := time.Now()

	server.lobby.Players["human-1"] = &Player{
		ID:        "human-1",
		Name:      "Pilot",
		Connected: true,
		Alive:     true,
		Mass:      server.cfg.StartingMass,
		Health:    server.maxHealthForMass(server.cfg.StartingMass),
	}

	server.resetMatchLocked(now)
	player := server.lobby.Players["human-1"]
	if player == nil {
		t.Fatalf("expected connected player to remain in lobby")
	}

	if !player.SpawnInvulnerableUntil.Equal(now.Add(server.cfg.SpawnInvulnerabilityDuration)) {
		t.Fatalf("spawn invulnerability expiry = %v, want %v", player.SpawnInvulnerableUntil, now.Add(server.cfg.SpawnInvulnerabilityDuration))
	}
	if !player.PendingSpawnSeparation {
		t.Fatalf("pending spawn separation = false, want true when fallback spawn is used")
	}
}

func TestEvictDisconnectedPlayersLockedAfterGrace(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()

	disconnected := &Player{
		ID:             "player-disconnected",
		Name:           "Ghost",
		Connected:      false,
		DisconnectedAt: now.Add(-11 * time.Second),
		Alive:          true,
		Mass:           server.cfg.StartingMass,
		Health:         server.maxHealthForMass(server.cfg.StartingMass),
	}
	connected := &Player{
		ID:        "player-connected",
		Name:      "Live",
		Connected: true,
		Alive:     true,
		Mass:      server.cfg.StartingMass,
		Health:    server.maxHealthForMass(server.cfg.StartingMass),
	}
	server.lobby.Players[disconnected.ID] = disconnected
	server.lobby.Players[connected.ID] = connected
	server.lobby.Projectiles = []*Projectile{
		{ID: "ghost-shot", OwnerID: disconnected.ID},
		{ID: "live-shot", OwnerID: connected.ID},
	}
	server.lobby.CrashPairs = map[string]time.Time{
		stablePairKey(disconnected.ID, connected.ID): now,
	}

	server.evictDisconnectedPlayersLocked(now)

	if _, ok := server.lobby.Players[disconnected.ID]; ok {
		t.Fatalf("expected disconnected player to be evicted")
	}
	if _, ok := server.lobby.Players[connected.ID]; !ok {
		t.Fatalf("expected connected player to remain")
	}
	if got := len(server.lobby.Projectiles); got != 1 {
		t.Fatalf("projectile count = %d, want 1", got)
	}
	if server.lobby.Projectiles[0].OwnerID != connected.ID {
		t.Fatalf("remaining projectile owner = %q, want %q", server.lobby.Projectiles[0].OwnerID, connected.ID)
	}
	if got := len(server.lobby.CrashPairs); got != 0 {
		t.Fatalf("crash pair count = %d, want 0", got)
	}
}

func TestEvictDisconnectedPlayersLockedRespectsGrace(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()

	player := &Player{
		ID:             "player-disconnected",
		Name:           "Ghost",
		Connected:      false,
		DisconnectedAt: now.Add(-9 * time.Second),
		Alive:          true,
		Mass:           server.cfg.StartingMass,
		Health:         server.maxHealthForMass(server.cfg.StartingMass),
	}
	server.lobby.Players[player.ID] = player

	server.evictDisconnectedPlayersLocked(now)

	if _, ok := server.lobby.Players[player.ID]; !ok {
		t.Fatalf("expected disconnected player to remain within grace period")
	}
}

func TestCustomBotProfilesCanBeSelectedWithoutCodeChanges(t *testing.T) {
	cfg := Config{
		LobbyID:                   "lobby-test",
		RedisAddr:                 "localhost:6379",
		TickRate:                  60,
		SnapshotRate:              20,
		MaxPlayers:                10,
		WorldWidth:                1200,
		WorldHeight:               800,
		GravityAccel:              2400,
		Drag:                      0.98,
		TerminalSpeed:             900,
		NumObjects:                8,
		StartingMass:              10,
		StartingHealth:            100,
		ProjectileSpeed:           1250,
		ProjectileDamage:          28,
		ProjectileRadius:          5,
		ShootCooldown:             250 * time.Millisecond,
		ProjectileTTL:             1200 * time.Millisecond,
		MatchDuration:             time.Minute,
		RespawnDelay:              2 * time.Second,
		BotFillDelay:              time.Hour,
		HealthTickThreshold:       2 * time.Second,
		BotDifficultyMode:         "fixed",
		BotDifficultyDistribution: "dodger",
		BotDifficultyProfiles: `{
			"dodger": {
				"behavior": "evasive",
				"avoidBorders": true,
				"threatRadius": 640,
				"wanderInset": 260
			}
		}`,
	}
	server := NewServer(cfg, log.New(io.Discard, "", 0))

	got := server.chooseBotLevelLocked()
	if got != BotLevel("dodger") {
		t.Fatalf("custom bot profile = %q, want %q", got, "dodger")
	}

	profile := server.botProfileForLocked(got)
	if profile.Behavior != botBehaviorEvasive {
		t.Fatalf("custom profile behavior = %q, want %q", profile.Behavior, botBehaviorEvasive)
	}
	if !profile.AvoidBorders {
		t.Fatalf("custom profile should enable border avoidance")
	}
	if math.Abs(profile.ThreatRadius-640) > 0.001 {
		t.Fatalf("custom profile threat radius = %v, want 640", profile.ThreatRadius)
	}
}

func TestEvasiveBotSteersAwayFromCorner(t *testing.T) {
	server := newClassicTestServer()
	server.rng = mathrand.New(mathrand.NewSource(1))
	now := time.Now()
	radius := server.radiusForMass(server.cfg.StartingMass)

	bot := &Player{
		ID:       "bot-evasive",
		IsBot:    true,
		Alive:    true,
		Mass:     server.cfg.StartingMass,
		Health:   server.maxHealthForMass(server.cfg.StartingMass),
		X:        radius,
		Y:        radius,
		BotLevel: BotLevelEvasive,
	}
	threat := &Player{
		ID:     "human-threat",
		Alive:  true,
		Mass:   server.cfg.StartingMass,
		Health: server.maxHealthForMass(server.cfg.StartingMass),
		X:      bot.X + 40,
		Y:      bot.Y + 40,
	}
	server.lobby.Players[bot.ID] = bot
	server.lobby.Players[threat.ID] = threat

	server.updateBotsLocked(now)

	if dx := math.Cos(bot.Input.Angle); dx <= 0 {
		t.Fatalf("bot steered into left wall: cos(angle) = %v, want > 0", dx)
	}
	if dy := math.Sin(bot.Input.Angle); dy <= 0 {
		t.Fatalf("bot steered into top wall: sin(angle) = %v, want > 0", dy)
	}
}

func TestCombatBotSteersBackInsideWhenFleeingBorder(t *testing.T) {
	server := newClassicTestServer()
	server.rng = mathrand.New(mathrand.NewSource(2))
	now := time.Now()
	radius := server.radiusForMass(server.cfg.StartingMass)

	bot := &Player{
		ID:       "bot-combat",
		IsBot:    true,
		Alive:    true,
		Mass:     server.cfg.StartingMass,
		Health:   server.maxHealthForMass(server.cfg.StartingMass),
		X:        server.cfg.WorldWidth - radius,
		Y:        server.cfg.WorldHeight / 2,
		BotLevel: BotLevelFull,
	}
	threat := &Player{
		ID:     "heavy-human",
		Alive:  true,
		Mass:   bot.Mass * 2,
		Health: server.maxHealthForMass(bot.Mass * 2),
		X:      bot.X - 50,
		Y:      bot.Y,
	}
	server.lobby.Players[bot.ID] = bot
	server.lobby.Players[threat.ID] = threat

	server.updateBotsLocked(now)

	if dx := math.Cos(bot.Input.Angle); dx >= 0 {
		t.Fatalf("combat bot steered into right wall: cos(angle) = %v, want < 0", dx)
	}
}

func TestWanderBotRetargetsAfterArrival(t *testing.T) {
	server := newClassicTestServer()
	server.rng = mathrand.New(mathrand.NewSource(3))
	now := time.Now()

	bot := &Player{
		ID:                    "bot-wander",
		IsBot:                 true,
		Alive:                 true,
		Mass:                  server.cfg.StartingMass,
		Health:                server.maxHealthForMass(server.cfg.StartingMass),
		X:                     420,
		Y:                     320,
		BotLevel:              BotLevelDummy,
		BotTargetX:            430,
		BotTargetY:            326,
		BotRetargetAt:         now.Add(time.Second),
		BotLastProgressAt:     now,
		BotLastTargetDistance: math.Hypot(10, 6),
	}
	server.lobby.Players[bot.ID] = bot

	oldTargetX, oldTargetY := bot.BotTargetX, bot.BotTargetY
	server.updateBotsLocked(now)

	if bot.BotTargetX == oldTargetX && bot.BotTargetY == oldTargetY {
		t.Fatalf("bot should retarget after arriving at waypoint")
	}
	if distance := math.Hypot(bot.BotTargetX-bot.X, bot.BotTargetY-bot.Y); distance < 200 {
		t.Fatalf("new wander target too close: got distance %v, want >= 200", distance)
	}
}

func TestWanderBotRetargetsWhenStalled(t *testing.T) {
	server := newClassicTestServer()
	server.rng = mathrand.New(mathrand.NewSource(4))
	now := time.Now()

	bot := &Player{
		ID:                    "bot-stalled",
		IsBot:                 true,
		Alive:                 true,
		Mass:                  server.cfg.StartingMass,
		Health:                server.maxHealthForMass(server.cfg.StartingMass),
		X:                     520,
		Y:                     340,
		BotLevel:              BotLevelEvasive,
		BotTargetX:            860,
		BotTargetY:            560,
		BotRetargetAt:         now.Add(time.Second),
		BotLastProgressAt:     now.Add(-2 * time.Second),
		BotLastTargetDistance: math.Hypot(860-520, 560-340),
	}
	server.lobby.Players[bot.ID] = bot

	oldTargetX, oldTargetY := bot.BotTargetX, bot.BotTargetY
	server.updateBotsLocked(now)

	if bot.BotTargetX == oldTargetX && bot.BotTargetY == oldTargetY {
		t.Fatalf("bot should retarget after stalling on a waypoint")
	}
}

func TestLevelZeroBotKeepsWaypointUntilArrival(t *testing.T) {
	server := newClassicTestServer()
	server.rng = mathrand.New(mathrand.NewSource(6))
	now := time.Now()

	bot := &Player{
		ID:                    "bot-level-zero",
		IsBot:                 true,
		Alive:                 true,
		Mass:                  server.cfg.StartingMass,
		Health:                server.maxHealthForMass(server.cfg.StartingMass),
		X:                     250,
		Y:                     250,
		BotLevel:              BotLevelDummy,
		BotTargetX:            900,
		BotTargetY:            600,
		BotRetargetAt:         now.Add(-time.Second),
		BotLastProgressAt:     now.Add(-5 * time.Second),
		BotLastTargetDistance: math.Hypot(900-250, 600-250),
	}
	server.lobby.Players[bot.ID] = bot

	oldTargetX, oldTargetY := bot.BotTargetX, bot.BotTargetY
	server.updateBotsLocked(now)

	if bot.BotTargetX != oldTargetX || bot.BotTargetY != oldTargetY {
		t.Fatalf("level 0 bot should keep current waypoint until arrival")
	}
	if bot.Input.Shoot {
		t.Fatalf("level 0 bot should never shoot")
	}
}

func TestCombatBotKeepsWaypointWhileEngaging(t *testing.T) {
	server := newClassicTestServer()
	server.rng = mathrand.New(mathrand.NewSource(5))
	now := time.Now()

	bot := &Player{
		ID:                    "bot-engage",
		IsBot:                 true,
		Alive:                 true,
		Mass:                  server.cfg.StartingMass,
		Health:                server.maxHealthForMass(server.cfg.StartingMass),
		X:                     500,
		Y:                     400,
		BotLevel:              BotLevelFull,
		BotTargetX:            900,
		BotTargetY:            650,
		BotRetargetAt:         now.Add(time.Second),
		BotLastProgressAt:     now.Add(-2 * time.Second),
		BotLastTargetDistance: math.Hypot(900-500, 650-400),
	}
	target := &Player{
		ID:     "target",
		Alive:  true,
		Mass:   server.cfg.StartingMass * 0.8,
		Health: server.maxHealthForMass(server.cfg.StartingMass) * 0.4,
		X:      bot.X + 120,
		Y:      bot.Y,
	}
	server.lobby.Players[bot.ID] = bot
	server.lobby.Players[target.ID] = target

	oldTargetX, oldTargetY := bot.BotTargetX, bot.BotTargetY
	server.updateBotsLocked(now)

	if !bot.Input.Shoot {
		t.Fatalf("combat bot should engage nearby target while traveling")
	}
	if bot.BotTargetX != oldTargetX || bot.BotTargetY != oldTargetY {
		t.Fatalf("combat engagement should not discard stored wander waypoint")
	}
	if dx := math.Cos(bot.Input.Angle); dx <= 0 {
		t.Fatalf("combat bot should steer toward nearby enemy while engaging: cos(angle) = %v", dx)
	}
}
