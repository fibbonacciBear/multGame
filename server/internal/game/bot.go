package game

import (
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

		target, distance := s.closestTargetLocked(player, livePlayers)
		object := s.bestCollectibleLocked(player)
		if target == nil || now.After(player.BotRetargetAt) {
			player.BotTargetX = s.randFloat(200, s.cfg.WorldWidth-200)
			player.BotTargetY = s.randFloat(200, s.cfg.WorldHeight-200)
			player.BotRetargetAt = now.Add(1500 * time.Millisecond)
		}

		targetX := player.BotTargetX
		targetY := player.BotTargetY
		shoot := false

		switch player.BotLevel {
		case BotLevelDummy:
			// Keep dummies moving but otherwise harmless.
			targetX = player.BotTargetX
			targetY = player.BotTargetY
		case BotLevelEvasive:
			if target != nil && distance < 550 {
				targetX = player.X - (target.X - player.X)
				targetY = player.Y - (target.Y - player.Y)
			} else if object != nil {
				targetX = object.X
				targetY = object.Y
			}
		case BotLevelNoviceCombat:
			targetX, targetY, shoot = s.botCombatDecisionLocked(player, target, object, distance, 0.18, now)
		default:
			targetX, targetY, shoot = s.botCombatDecisionLocked(player, target, object, distance, 0.05, now)
		}

		angle := math.Atan2(targetY-player.Y, targetX-player.X)
		if player.BotLevel == BotLevelNoviceCombat {
			angle += s.randFloat(-0.16, 0.16)
		}
		player.Input = InputState{
			Angle:    angle,
			Strength: 0.9,
			Shoot:    shoot && !s.isInvulnerable(player, now),
		}
	}
}

func (s *Server) botCombatDecisionLocked(
	player *Player,
	target *Player,
	object *Collectible,
	distance float64,
	aimNoise float64,
	now time.Time,
) (float64, float64, bool) {
	targetX := player.BotTargetX
	targetY := player.BotTargetY
	shoot := false

	if object != nil {
		targetX = object.X
		targetY = object.Y
	}

	if target == nil {
		return targetX, targetY, false
	}

	projectedCrashDamage := s.crashDamageForPlayer(player)
	if projectedCrashDamage >= player.Health || target.Mass > player.Mass*1.2 {
		targetX = player.X - (target.X - player.X)
		targetY = player.Y - (target.Y - player.Y)
		return targetX, targetY, false
	}

	if !s.isCrashPairOnCooldown(player.ID, target.ID, now) && target.Health < s.maxHealthForMass(target.Mass)*0.45 {
		targetX = target.X
		targetY = target.Y
	}

	if target.Mass <= player.Mass*1.15 {
		targetX = target.X + s.randFloat(-aimNoise, aimNoise)*distance
		targetY = target.Y + s.randFloat(-aimNoise, aimNoise)*distance
		shoot = distance < 700
	}

	if object != nil {
		objectScore := object.Mass / math.Max(math.Hypot(object.X-player.X, object.Y-player.Y), 120)
		targetScore := s.killMassTransfer(target.Mass) / math.Max(distance, 120)
		if objectScore > targetScore && distance > 260 {
			targetX = object.X
			targetY = object.Y
			shoot = false
		}
	}

	return targetX, targetY, shoot
}

func (s *Server) bestCollectibleLocked(player *Player) *Collectible {
	var best *Collectible
	bestScore := -1.0
	for _, object := range s.lobby.Objects {
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
		Name:          fmt.Sprintf("Bot-%02d", s.rng.Intn(90)+10),
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
		return parseFixedBotLevel(s.cfg.BotDifficultyDistribution)
	case "adaptive":
		connectedHumans := 0
		for _, player := range s.lobby.Players {
			if !player.IsBot && player.Connected {
				connectedHumans++
			}
		}
		if connectedHumans <= 1 {
			return BotLevelEvasive
		}
		if connectedHumans >= 4 {
			return BotLevelFull
		}
		return s.chooseWeightedBotLevel(s.cfg.BotDifficultyDistribution)
	default:
		return s.chooseWeightedBotLevel(s.cfg.BotDifficultyDistribution)
	}
}

func parseFixedBotLevel(value string) BotLevel {
	switch BotLevel(strings.TrimSpace(value)) {
	case BotLevelDummy, BotLevelEvasive, BotLevelNoviceCombat, BotLevelFull:
		return BotLevel(strings.TrimSpace(value))
	default:
		return BotLevelFull
	}
}

func (s *Server) chooseWeightedBotLevel(distribution string) BotLevel {
	type weightedLevel struct {
		level  BotLevel
		weight int
	}

	options := make([]weightedLevel, 0, 4)
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
		level := parseFixedBotLevel(sides[0])
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

	roll := s.rng.Intn(totalWeight)
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
