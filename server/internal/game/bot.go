package game

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type BotLevel string

const (
	BotLevelDummy        BotLevel = "L0"
	BotLevelEvasive      BotLevel = "L1"
	BotLevelNoviceCombat BotLevel = "L2"
	BotLevelFull         BotLevel = "L3"
)

type botBehavior string

const (
	botBehaviorWander  botBehavior = "wander"
	botBehaviorEvasive botBehavior = "evasive"
	botBehaviorCombat  botBehavior = "combat"
)

const (
	botWanderTimeout           = 3 * time.Second
	botWanderStallWindow       = 1100 * time.Millisecond
	botWanderCandidateAttempts = 8
	botCollectibleCooldown     = 20 * time.Second
)

type botProfile struct {
	ID                     BotLevel
	Behavior               botBehavior
	CanShoot               bool
	AvoidBorders           bool
	ThreatRadius           float64
	AimNoise               float64
	MoveNoise              float64
	BorderMargin           float64
	WanderInset            float64
	InputStrength          float64
	FleeMassRatio          float64
	AimMassRatio           float64
	ShootRange             float64
	FinishHealthPct        float64
	CollectibleMinDistance float64
}

type botProfileConfig struct {
	Behavior               *string  `json:"behavior"`
	CanShoot               *bool    `json:"canShoot"`
	AvoidBorders           *bool    `json:"avoidBorders"`
	ThreatRadius           *float64 `json:"threatRadius"`
	AimNoise               *float64 `json:"aimNoise"`
	MoveNoise              *float64 `json:"moveNoise"`
	BorderMargin           *float64 `json:"borderMargin"`
	WanderInset            *float64 `json:"wanderInset"`
	InputStrength          *float64 `json:"inputStrength"`
	FleeMassRatio          *float64 `json:"fleeMassRatio"`
	AimMassRatio           *float64 `json:"aimMassRatio"`
	ShootRange             *float64 `json:"shootRange"`
	FinishHealthPct        *float64 `json:"finishHealthPct"`
	CollectibleMinDistance *float64 `json:"collectibleMinDistance"`
}

func defaultBotProfiles() map[BotLevel]botProfile {
	return map[BotLevel]botProfile{
		BotLevelDummy: normalizeBotProfile(botProfile{
			ID:          BotLevelDummy,
			Behavior:    botBehaviorWander,
			WanderInset: 200,
		}),
		BotLevelEvasive: normalizeBotProfile(botProfile{
			ID:           BotLevelEvasive,
			Behavior:     botBehaviorEvasive,
			AvoidBorders: true,
			ThreatRadius: 550,
			BorderMargin: 220,
			WanderInset:  220,
		}),
		BotLevelNoviceCombat: normalizeBotProfile(botProfile{
			ID:                     BotLevelNoviceCombat,
			Behavior:               botBehaviorCombat,
			CanShoot:               true,
			AvoidBorders:           true,
			AimNoise:               0.18,
			MoveNoise:              0.16,
			BorderMargin:           220,
			WanderInset:            220,
			ThreatRadius:           550,
			FleeMassRatio:          1.2,
			AimMassRatio:           1.15,
			ShootRange:             700,
			FinishHealthPct:        0.45,
			CollectibleMinDistance: 260,
		}),
		BotLevelFull: normalizeBotProfile(botProfile{
			ID:                     BotLevelFull,
			Behavior:               botBehaviorCombat,
			CanShoot:               true,
			AvoidBorders:           true,
			AimNoise:               0.05,
			MoveNoise:              0,
			BorderMargin:           220,
			WanderInset:            220,
			ThreatRadius:           550,
			FleeMassRatio:          1.2,
			AimMassRatio:           1.15,
			ShootRange:             700,
			FinishHealthPct:        0.45,
			CollectibleMinDistance: 260,
		}),
	}
}

func genericBotProfile(id BotLevel) botProfile {
	return normalizeBotProfile(botProfile{
		ID:                     id,
		WanderInset:            200,
		ThreatRadius:           550,
		BorderMargin:           220,
		InputStrength:          0.9,
		FleeMassRatio:          1.2,
		AimMassRatio:           1.15,
		ShootRange:             700,
		FinishHealthPct:        0.45,
		CollectibleMinDistance: 260,
	})
}

func normalizeBotProfile(profile botProfile) botProfile {
	if profile.ThreatRadius <= 0 {
		profile.ThreatRadius = 550
	}
	if profile.BorderMargin <= 0 {
		profile.BorderMargin = 220
	}
	if profile.InputStrength <= 0 {
		profile.InputStrength = 0.9
	}
	if profile.FleeMassRatio <= 0 {
		profile.FleeMassRatio = 1.2
	}
	if profile.AimMassRatio <= 0 {
		profile.AimMassRatio = 1.15
	}
	if profile.ShootRange <= 0 {
		profile.ShootRange = 700
	}
	if profile.FinishHealthPct <= 0 {
		profile.FinishHealthPct = 0.45
	}
	if profile.CollectibleMinDistance <= 0 {
		profile.CollectibleMinDistance = 260
	}
	if profile.WanderInset <= 0 {
		profile.WanderInset = 200
		if profile.AvoidBorders {
			profile.WanderInset = profile.BorderMargin
		}
	}
	return profile
}

func loadBotProfiles(raw string) (map[BotLevel]botProfile, error) {
	profiles := defaultBotProfiles()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return profiles, nil
	}

	var overrides map[string]botProfileConfig
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return nil, err
	}

	for idText, override := range overrides {
		id := BotLevel(strings.TrimSpace(idText))
		if id == "" {
			return nil, fmt.Errorf("empty bot profile id")
		}
		base, exists := profiles[id]
		if !exists {
			base = genericBotProfile(id)
		}
		profile, err := applyBotProfileConfig(base, override, !exists)
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", id, err)
		}
		profiles[id] = profile
	}
	return profiles, nil
}

func applyBotProfileConfig(base botProfile, cfg botProfileConfig, requireBehavior bool) (botProfile, error) {
	if cfg.Behavior != nil {
		base.Behavior = botBehavior(strings.ToLower(strings.TrimSpace(*cfg.Behavior)))
	}
	if cfg.CanShoot != nil {
		base.CanShoot = *cfg.CanShoot
	}
	if cfg.AvoidBorders != nil {
		base.AvoidBorders = *cfg.AvoidBorders
	}
	if cfg.ThreatRadius != nil {
		base.ThreatRadius = *cfg.ThreatRadius
	}
	if cfg.AimNoise != nil {
		base.AimNoise = *cfg.AimNoise
	}
	if cfg.MoveNoise != nil {
		base.MoveNoise = *cfg.MoveNoise
	}
	if cfg.BorderMargin != nil {
		base.BorderMargin = *cfg.BorderMargin
	}
	if cfg.WanderInset != nil {
		base.WanderInset = *cfg.WanderInset
	}
	if cfg.InputStrength != nil {
		base.InputStrength = *cfg.InputStrength
	}
	if cfg.FleeMassRatio != nil {
		base.FleeMassRatio = *cfg.FleeMassRatio
	}
	if cfg.AimMassRatio != nil {
		base.AimMassRatio = *cfg.AimMassRatio
	}
	if cfg.ShootRange != nil {
		base.ShootRange = *cfg.ShootRange
	}
	if cfg.FinishHealthPct != nil {
		base.FinishHealthPct = *cfg.FinishHealthPct
	}
	if cfg.CollectibleMinDistance != nil {
		base.CollectibleMinDistance = *cfg.CollectibleMinDistance
	}

	if requireBehavior && base.Behavior == "" {
		return botProfile{}, fmt.Errorf("behavior is required for new profiles")
	}
	switch base.Behavior {
	case botBehaviorWander, botBehaviorEvasive, botBehaviorCombat:
	default:
		return botProfile{}, fmt.Errorf("unsupported behavior %q", base.Behavior)
	}
	return normalizeBotProfile(base), nil
}

func (s *Server) updateBotsLocked(now time.Time) {
	livePlayers := make([]*Player, 0, len(s.lobby.Players))
	for _, player := range s.lobby.Players {
		if player.Alive {
			livePlayers = append(livePlayers, player)
		}
	}

	for _, player := range s.lobby.Players {
		if !player.IsBot || !player.Alive {
			continue
		}

		profile := s.botProfileForLocked(player.BotLevel)
		s.ensureBotWanderTargetLocked(player, profile, now)
		target, distance := s.closestTargetLocked(player, livePlayers)
		object := s.botCollectibleTargetLocked(player, profile, now)

		targetX, targetY, shoot, usingWander := s.botDecisionLocked(player, profile, target, object, distance, now)
		if usingWander && s.shouldRetargetBotWanderLocked(player, profile, now) {
			s.retargetBotLocked(player, profile, now)
			targetX = player.BotTargetX
			targetY = player.BotTargetY
		}
		targetX, targetY = s.applyBotCornerEscapeOverrideLocked(player, profile, targetX, targetY)
		targetX, targetY = s.applyBotBorderAvoidanceLocked(player, profile, targetX, targetY)
		angle := s.botSteeringAngleLocked(player, profile, targetX, targetY)
		player.Input = InputState{
			Angle:    angle,
			Strength: profile.InputStrength,
			Shoot:    shoot && profile.CanShoot && !s.isInvulnerable(player, now),
		}
	}
}

func (s *Server) ensureBotWanderTargetLocked(player *Player, profile botProfile, now time.Time) {
	if player.BotRetargetAt.IsZero() {
		s.retargetBotLocked(player, profile, now)
		return
	}
	if profile.Behavior != botBehaviorWander && now.After(player.BotRetargetAt) {
		s.retargetBotLocked(player, profile, now)
	}
}

func (s *Server) shouldRetargetBotWanderLocked(player *Player, profile botProfile, now time.Time) bool {
	distance := math.Hypot(player.BotTargetX-player.X, player.BotTargetY-player.Y)
	if distance <= s.botWanderArrivalRadiusLocked(player) {
		return true
	}
	if profile.Behavior == botBehaviorWander {
		return false
	}

	if player.BotLastProgressAt.IsZero() || player.BotLastTargetDistance <= 0 {
		player.BotLastProgressAt = now
		player.BotLastTargetDistance = distance
		return false
	}

	if distance <= player.BotLastTargetDistance-s.botWanderProgressThresholdLocked(player) {
		player.BotLastProgressAt = now
		player.BotLastTargetDistance = distance
		return false
	}

	return now.Sub(player.BotLastProgressAt) >= botWanderStallWindow
}

func (s *Server) botWanderArrivalRadiusLocked(player *Player) float64 {
	return math.Max(s.radiusForMass(player.Mass)*2, 45)
}

func (s *Server) botWanderProgressThresholdLocked(player *Player) float64 {
	return math.Max(s.radiusForMass(player.Mass)*0.8, 24)
}

func (s *Server) botMinimumWanderDistanceLocked(player *Player, profile botProfile) float64 {
	return math.Max(math.Max(profile.BorderMargin*1.1, 240), s.radiusForMass(player.Mass)*10)
}

func (s *Server) retargetBotLocked(player *Player, profile botProfile, now time.Time) {
	player.BotTargetX, player.BotTargetY = s.randomBotWanderTargetLocked(player, profile)
	player.BotRetargetAt = now.Add(botWanderTimeout)
	player.BotLastProgressAt = now
	player.BotLastTargetDistance = math.Hypot(player.BotTargetX-player.X, player.BotTargetY-player.Y)
}

func (s *Server) randomBotWanderTargetLocked(player *Player, profile botProfile) (float64, float64) {
	minX, maxX, minY, maxY := s.botWanderBoundsLocked(player, profile)

	minDistance := s.botMinimumWanderDistanceLocked(player, profile)
	bestX, bestY := s.randFloat(minX, maxX), s.randFloat(minY, maxY)
	bestDistance := math.Hypot(bestX-player.X, bestY-player.Y)
	for attempt := 1; attempt < botWanderCandidateAttempts; attempt++ {
		candidateX := s.randFloat(minX, maxX)
		candidateY := s.randFloat(minY, maxY)
		distance := math.Hypot(candidateX-player.X, candidateY-player.Y)
		if distance > bestDistance {
			bestX, bestY = candidateX, candidateY
			bestDistance = distance
		}
		if distance >= minDistance {
			return candidateX, candidateY
		}
	}
	return bestX, bestY
}

func (s *Server) botDecisionLocked(
	player *Player,
	profile botProfile,
	target *Player,
	object *Collectible,
	distance float64,
	now time.Time,
) (float64, float64, bool, bool) {
	switch profile.Behavior {
	case botBehaviorWander:
		return player.BotTargetX, player.BotTargetY, false, true
	case botBehaviorEvasive:
		return s.botEvasiveDecisionLocked(player, profile, target, object, distance)
	case botBehaviorCombat:
		return s.botCombatDecisionLocked(player, profile, target, object, distance, now)
	default:
		return player.BotTargetX, player.BotTargetY, false, true
	}
}

func (s *Server) botEvasiveDecisionLocked(
	player *Player,
	profile botProfile,
	target *Player,
	object *Collectible,
	distance float64,
) (float64, float64, bool, bool) {
	targetX := player.BotTargetX
	targetY := player.BotTargetY
	if target != nil && distance < profile.ThreatRadius {
		targetX, targetY = mirroredTarget(player, target)
		return targetX, targetY, false, false
	}
	if object != nil {
		return object.X, object.Y, false, false
	}
	return targetX, targetY, false, true
}

func mirroredTarget(player *Player, target *Player) (float64, float64) {
	return player.X - (target.X - player.X), player.Y - (target.Y - player.Y)
}

func (s *Server) botSteeringAngleLocked(player *Player, profile botProfile, targetX, targetY float64) float64 {
	dx := targetX - player.X
	dy := targetY - player.Y
	if math.Hypot(dx, dy) < 1 {
		dx = s.cfg.WorldWidth/2 - player.X
		dy = s.cfg.WorldHeight/2 - player.Y
	}
	angle := math.Atan2(dy, dx)
	if profile.MoveNoise > 0 {
		angle += s.randFloat(-profile.MoveNoise, profile.MoveNoise)
	}
	return angle
}

func (s *Server) applyBotCornerEscapeOverrideLocked(player *Player, profile botProfile, targetX, targetY float64) (float64, float64) {
	if escapeX, escapeY, recovering := s.botCornerRecoveryTargetLocked(player, profile); recovering {
		return escapeX, escapeY
	}
	if profile.Behavior != botBehaviorWander || profile.AvoidBorders {
		return targetX, targetY
	}

	radius := s.radiusForMass(player.Mass)
	cornerMargin := math.Max(radius*1.5, 60)
	nearLeft := player.X <= radius+cornerMargin
	nearRight := player.X >= s.cfg.WorldWidth-radius-cornerMargin
	nearTop := player.Y <= radius+cornerMargin
	nearBottom := player.Y >= s.cfg.WorldHeight-radius-cornerMargin
	if !(nearLeft || nearRight) || !(nearTop || nearBottom) {
		return targetX, targetY
	}

	minX, maxX, minY, maxY := s.botBoundsForInsetLocked(player, math.Max(profile.WanderInset, 120))
	escapeX, escapeY := targetX, targetY
	if nearLeft {
		escapeX = minX
	} else if nearRight {
		escapeX = maxX
	}
	if nearTop {
		escapeY = minY
	} else if nearBottom {
		escapeY = maxY
	}
	return escapeX, escapeY
}

func (s *Server) botCornerRecoveryTargetLocked(player *Player, profile botProfile) (float64, float64, bool) {
	if !profile.AvoidBorders || profile.BorderMargin <= 0 {
		player.BotCornerRecovering = false
		return 0, 0, false
	}

	minX, maxX, minY, maxY := s.botSafeBoundsLocked(player, profile)
	enterBuffer := math.Max(profile.BorderMargin*0.15, 24)
	exitBuffer := math.Max(profile.BorderMargin*0.45, 80)
	buffer := enterBuffer
	if player.BotCornerRecovering {
		buffer = exitBuffer
	}

	nearLeft := player.X <= minX+buffer
	nearRight := player.X >= maxX-buffer
	nearTop := player.Y <= minY+buffer
	nearBottom := player.Y >= maxY-buffer
	recovering := (nearLeft || nearRight) && (nearTop || nearBottom)
	player.BotCornerRecovering = recovering
	if !recovering {
		return 0, 0, false
	}

	extraInset := profile.BorderMargin + math.Max(profile.BorderMargin*0.6, 120)
	recoverMinX, recoverMaxX, recoverMinY, recoverMaxY := s.botBoundsForInsetLocked(player, extraInset)
	escapeX := s.cfg.WorldWidth / 2
	escapeY := s.cfg.WorldHeight / 2
	if nearLeft {
		escapeX = recoverMinX
	} else if nearRight {
		escapeX = recoverMaxX
	}
	if nearTop {
		escapeY = recoverMinY
	} else if nearBottom {
		escapeY = recoverMaxY
	}
	return escapeX, escapeY, true
}

func (s *Server) applyBotBorderAvoidanceLocked(player *Player, profile botProfile, targetX, targetY float64) (float64, float64) {
	if !profile.AvoidBorders || profile.BorderMargin <= 0 {
		return targetX, targetY
	}

	minX, maxX, minY, maxY := s.botSafeBoundsLocked(player, profile)

	safeTargetX := clamp(targetX, minX, maxX)
	safeTargetY := clamp(targetY, minY, maxY)
	escapeX := clamp(player.X, minX, maxX)
	escapeY := clamp(player.Y, minY, maxY)

	pressureX := 0.0
	if player.X < minX {
		pressureX = (minX - player.X) / profile.BorderMargin
	} else if player.X > maxX {
		pressureX = (player.X - maxX) / profile.BorderMargin
	}
	pressureY := 0.0
	if player.Y < minY {
		pressureY = (minY - player.Y) / profile.BorderMargin
	} else if player.Y > maxY {
		pressureY = (player.Y - maxY) / profile.BorderMargin
	}

	pressure := clamp(math.Hypot(pressureX, pressureY), 0, 1)
	adjustedX := lerp(safeTargetX, escapeX, pressure)
	adjustedY := lerp(safeTargetY, escapeY, pressure)
	if math.Hypot(adjustedX-player.X, adjustedY-player.Y) >= 1 {
		return adjustedX, adjustedY
	}

	centerX := s.cfg.WorldWidth / 2
	centerY := s.cfg.WorldHeight / 2
	if pressure > 0 {
		return lerp(safeTargetX, centerX, pressure), lerp(safeTargetY, centerY, pressure)
	}
	return safeTargetX, safeTargetY
}

func (s *Server) botWanderBoundsLocked(player *Player, profile botProfile) (float64, float64, float64, float64) {
	inset := profile.WanderInset
	if profile.AvoidBorders {
		inset = math.Max(inset, profile.BorderMargin)
	}
	return s.botBoundsForInsetLocked(player, inset)
}

func (s *Server) botSafeBoundsLocked(player *Player, profile botProfile) (float64, float64, float64, float64) {
	return s.botBoundsForInsetLocked(player, profile.BorderMargin)
}

func (s *Server) botBoundsForInsetLocked(player *Player, inset float64) (float64, float64, float64, float64) {
	radius := s.radiusForMass(player.Mass)
	inset = math.Max(inset, 0)
	minX := radius + inset
	maxX := s.cfg.WorldWidth - radius - inset
	minY := radius + inset
	maxY := s.cfg.WorldHeight - radius - inset
	if minX > maxX {
		minX, maxX = radius, s.cfg.WorldWidth-radius
	}
	if minY > maxY {
		minY, maxY = radius, s.cfg.WorldHeight-radius
	}
	return minX, maxX, minY, maxY
}

func lerp(start, end, amount float64) float64 {
	return start + (end-start)*amount
}

func (s *Server) botCombatDecisionLocked(
	player *Player,
	profile botProfile,
	target *Player,
	object *Collectible,
	distance float64,
	now time.Time,
) (float64, float64, bool, bool) {
	targetX := player.BotTargetX
	targetY := player.BotTargetY
	shoot := false
	usingWander := true

	if object != nil {
		targetX = object.X
		targetY = object.Y
		usingWander = false
	}

	if target == nil {
		return targetX, targetY, false, usingWander
	}

	projectedCrashDamage := s.crashDamageForPlayer(player)
	if projectedCrashDamage >= player.Health || target.Mass > player.Mass*profile.FleeMassRatio {
		targetX, targetY = mirroredTarget(player, target)
		return targetX, targetY, false, false
	}

	if !s.isCrashPairOnCooldown(player.ID, target.ID, now) && target.Health < s.maxHealthForMass(target.Mass)*profile.FinishHealthPct {
		targetX = target.X
		targetY = target.Y
		usingWander = false
	}

	if target.Mass <= player.Mass*profile.AimMassRatio {
		targetX = target.X + s.randFloat(-profile.AimNoise, profile.AimNoise)*distance
		targetY = target.Y + s.randFloat(-profile.AimNoise, profile.AimNoise)*distance
		shoot = distance < profile.ShootRange
		usingWander = false
	}

	if object != nil {
		objectScore := object.Mass / math.Max(math.Hypot(object.X-player.X, object.Y-player.Y), 120)
		targetScore := s.killMassTransfer(target.Mass) / math.Max(distance, 120)
		if objectScore > targetScore && distance > profile.CollectibleMinDistance {
			targetX = object.X
			targetY = object.Y
			shoot = false
			usingWander = false
		}
	}

	return targetX, targetY, shoot, usingWander
}

func (s *Server) botProfileForLocked(level BotLevel) botProfile {
	if profile, ok := s.botProfiles[level]; ok {
		return profile
	}
	if profile, ok := s.botProfiles[BotLevelFull]; ok {
		return profile
	}
	return defaultBotProfiles()[BotLevelFull]
}

func (s *Server) botCollectibleTargetLocked(player *Player, profile botProfile, now time.Time) *Collectible {
	if profile.Behavior == botBehaviorWander {
		player.BotCollectibleTargetID = ""
		return nil
	}
	if !player.BotCollectibleCooldownUntil.IsZero() && now.Before(player.BotCollectibleCooldownUntil) {
		player.BotCollectibleTargetID = ""
		return nil
	}
	if player.BotCollectibleTargetID != "" {
		if object := s.collectibleByIDLocked(player.BotCollectibleTargetID); s.isCollectibleSafeForBotLocked(player, profile, object) {
			return object
		}
		player.BotCollectibleTargetID = ""
	}

	object := s.bestCollectibleLocked(player, profile)
	if object != nil {
		player.BotCollectibleTargetID = object.ID
	}
	return object
}

func (s *Server) collectibleByIDLocked(id string) *Collectible {
	for _, object := range s.lobby.Objects {
		if object.ID == id {
			return object
		}
	}
	return nil
}

func (s *Server) isCollectibleSafeForBotLocked(player *Player, profile botProfile, object *Collectible) bool {
	if object == nil {
		return false
	}
	if !profile.AvoidBorders || profile.BorderMargin <= 0 {
		return true
	}
	minX, maxX, minY, maxY := s.botSafeBoundsLocked(player, profile)
	return object.X >= minX && object.X <= maxX && object.Y >= minY && object.Y <= maxY
}

func (s *Server) bestCollectibleLocked(player *Player, profile botProfile) *Collectible {
	var best *Collectible
	bestScore := -1.0
	for _, object := range s.lobby.Objects {
		if !s.isCollectibleSafeForBotLocked(player, profile, object) {
			continue
		}
		distance := math.Hypot(object.X-player.X, object.Y-player.Y)
		score := object.Mass / math.Max(distance, 100)
		if score > bestScore {
			bestScore = score
			best = object
		}
	}
	return best
}

func (s *Server) closestTargetLocked(source *Player, players []*Player) (*Player, float64) {
	var target *Player
	bestDistance := math.MaxFloat64
	for _, candidate := range players {
		if candidate.ID == source.ID || !candidate.Alive {
			continue
		}
		distance := math.Hypot(candidate.X-source.X, candidate.Y-source.Y)
		if distance < bestDistance {
			bestDistance = distance
			target = candidate
		}
	}
	return target, bestDistance
}

func (s *Server) newBotLocked(now time.Time) *Player {
	bot := &Player{
		ID:            randomID("bot"),
		Name:          fmt.Sprintf("Bot-%02d", s.randomIntnLocked(90)+10),
		Color:         s.randomColor(),
		SpriteVariant: s.randomSpriteVariant(),
		IsBot:         true,
		Alive:         true,
		Connected:     false,
		Mass:          s.cfg.StartingMass,
		Health:        s.maxHealthForMass(s.cfg.StartingMass),
		LastShotAt:    now.Add(-s.cfg.ShootCooldown),
		BotLevel:      s.chooseBotLevelLocked(),
	}
	usedFallback := s.spawnPlayerAtRandomPositionLocked(bot)
	s.applySpawnSafetyLocked(bot, now, usedFallback)
	return bot
}

func (s *Server) chooseBotLevelLocked() BotLevel {
	switch strings.ToLower(strings.TrimSpace(s.cfg.BotDifficultyMode)) {
	case "fixed":
		return s.parseFixedBotLevel(s.cfg.BotDifficultyDistribution)
	case "adaptive":
		connectedHumans := 0
		for _, player := range s.lobby.Players {
			if !player.IsBot && player.Connected {
				connectedHumans++
			}
		}
		if connectedHumans <= 1 {
			return s.parseFixedBotLevel(s.cfg.BotDifficultyAdaptiveLow)
		}
		if connectedHumans >= 4 {
			return s.parseFixedBotLevel(s.cfg.BotDifficultyAdaptiveHigh)
		}
		return s.chooseWeightedBotLevel(s.cfg.BotDifficultyDistribution)
	default:
		return s.chooseWeightedBotLevel(s.cfg.BotDifficultyDistribution)
	}
}

func (s *Server) parseFixedBotLevel(value string) BotLevel {
	level := BotLevel(strings.TrimSpace(value))
	if level == "" {
		return BotLevelFull
	}
	if _, ok := s.botProfiles[level]; ok {
		return level
	}
	return BotLevelFull
}

func (s *Server) chooseWeightedBotLevel(distribution string) BotLevel {
	type weightedLevel struct {
		level  BotLevel
		weight int
	}

	options := make([]weightedLevel, 0, len(s.botProfiles))
	totalWeight := 0
	for _, part := range strings.Split(distribution, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		sides := strings.SplitN(part, ":", 2)
		if len(sides) != 2 {
			continue
		}
		level := BotLevel(strings.TrimSpace(sides[0]))
		if _, ok := s.botProfiles[level]; !ok {
			continue
		}
		weight := envIntFromString(strings.TrimSpace(sides[1]), 0)
		if weight <= 0 {
			continue
		}
		options = append(options, weightedLevel{level: level, weight: weight})
		totalWeight += weight
	}
	if totalWeight <= 0 {
		return BotLevelFull
	}

	roll := s.randomIntnLocked(totalWeight)
	running := 0
	for _, option := range options {
		running += option.weight
		if roll < running {
			return option.level
		}
	}
	return BotLevelFull
}

func envIntFromString(value string, fallback int) int {
	result := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return fallback
		}
		result = result*10 + int(ch-'0')
	}
	if result == 0 {
		return fallback
	}
	return result
}
