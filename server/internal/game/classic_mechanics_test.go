package game

import (
	"io"
	"log"
	"math"
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
		Mass:   0.8,
	}}

	server.resolveObjectCollisionsLocked(now)

	if player.Mass <= server.cfg.StartingMass {
		t.Fatalf("player mass did not increase during invulnerable pickup: got %v", player.Mass)
	}
	if got := server.lobby.Objects[0].ID; got == "obj-1" {
		t.Fatalf("object was not respawned after pickup")
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
