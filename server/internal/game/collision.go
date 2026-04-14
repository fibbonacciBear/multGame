package game

import (
	"fmt"
	"math"
	"time"
)

const (
	spawnSeparationSteps   = 8
	spawnSeparationMaxPush = 24.0
)

type combatSourceKind string

const (
	combatSourceCrash      combatSourceKind = "crash"
	combatSourceProjectile combatSourceKind = "projectile"
)

type combatSource struct {
	killerID string
	kind     combatSourceKind
	reason   string
}

type crashPair struct {
	leftID  string
	rightID string
}

func (s *Server) resolveObjectCollisionsLocked(_ time.Time) {
	for _, player := range s.lobby.Players {
		if !player.Alive {
			continue
		}

		playerRadius := s.radiusForMass(player.Mass)
		for index := range s.lobby.Objects {
			object := s.lobby.Objects[index]
			dx := player.X - object.X
			dy := player.Y - object.Y
			if math.Hypot(dx, dy) >= playerRadius+object.Radius {
				continue
			}

			s.setPlayerMassPreservingHealth(player, player.Mass+object.Mass)
			s.lobby.Objects[index] = s.spawnObjectLocked()
		}
	}
}

func (s *Server) spawnObjectLocked() *Collectible {
	mass := s.randFloat(0.35, 1.15)
	radius := 4 + mass*6
	return &Collectible{
		ID:     randomID("obj"),
		X:      s.randFloat(radius, s.cfg.WorldWidth-radius),
		Y:      s.randFloat(radius, s.cfg.WorldHeight-radius),
		Radius: radius,
		Mass:   mass,
	}
}

func (s *Server) resolveCombatLocked(now time.Time) {
	damage := make(map[string]float64)
	sources := make(map[string]combatSource)
	combatants := make(map[string]struct{})
	knockbacks := make([]crashPair, 0)

	s.collectCrashDamageLocked(now, damage, sources, combatants, &knockbacks)
	s.collectProjectileDamageLocked(now, damage, sources, combatants)
	s.applyCombatResolutionLocked(now, damage, sources, combatants, knockbacks)
}

func (s *Server) collectCrashDamageLocked(
	now time.Time,
	damage map[string]float64,
	sources map[string]combatSource,
	combatants map[string]struct{},
	knockbacks *[]crashPair,
) {
	s.pruneCrashPairsLocked(now)

	livePlayers := make([]*Player, 0, len(s.lobby.Players))
	for _, player := range s.lobby.Players {
		if player.Alive {
			livePlayers = append(livePlayers, player)
		}
	}

	for i := 0; i < len(livePlayers); i++ {
		for j := i + 1; j < len(livePlayers); j++ {
			left := livePlayers[i]
			right := livePlayers[j]
			if !left.Alive || !right.Alive {
				continue
			}
			if s.isInvulnerable(left, now) || s.isInvulnerable(right, now) {
				continue
			}
			if s.isCrashPairOnCooldown(left.ID, right.ID, now) {
				continue
			}

			minDistance := s.radiusForMass(left.Mass) + s.radiusForMass(right.Mass)
			if math.Hypot(left.X-right.X, left.Y-right.Y) >= minDistance {
				continue
			}

			s.lobby.CrashPairs[stablePairKey(left.ID, right.ID)] = now
			damage[left.ID] += s.crashDamageForPlayer(right)
			damage[right.ID] += s.crashDamageForPlayer(left)
			sources[left.ID] = combatSource{
				killerID: right.ID,
				kind:     combatSourceCrash,
				reason:   fmt.Sprintf("rammed by %s", right.Name),
			}
			sources[right.ID] = combatSource{
				killerID: left.ID,
				kind:     combatSourceCrash,
				reason:   fmt.Sprintf("rammed by %s", left.Name),
			}
			combatants[left.ID] = struct{}{}
			combatants[right.ID] = struct{}{}
			*knockbacks = append(*knockbacks, crashPair{leftID: left.ID, rightID: right.ID})
			CrashContacts.Inc()
		}
	}
}

func (s *Server) collectProjectileDamageLocked(
	now time.Time,
	damage map[string]float64,
	sources map[string]combatSource,
	combatants map[string]struct{},
) {
	projectiles := s.lobby.Projectiles[:0]
	for _, projectile := range s.lobby.Projectiles {
		var target *Player
		bestDistance := math.MaxFloat64
		for _, player := range s.lobby.Players {
			if !player.Alive || player.ID == projectile.OwnerID {
				continue
			}
			if s.isInvulnerable(player, now) {
				continue
			}

			minDistance := s.radiusForMass(player.Mass) + projectile.Radius
			distance := math.Hypot(player.X-projectile.X, player.Y-projectile.Y)
			if distance >= minDistance {
				continue
			}
			if distance < bestDistance {
				bestDistance = distance
				target = player
			}
		}

		if target != nil {
			damage[target.ID] += projectile.Damage
			if owner := s.lobby.Players[projectile.OwnerID]; owner != nil {
				sources[target.ID] = combatSource{
					killerID: owner.ID,
					kind:     combatSourceProjectile,
					reason:   fmt.Sprintf("shot down by %s", owner.Name),
				}
				combatants[owner.ID] = struct{}{}
			} else {
				sources[target.ID] = combatSource{
					kind:   combatSourceProjectile,
					reason: "shot down",
				}
			}
			combatants[target.ID] = struct{}{}
		} else {
			projectiles = append(projectiles, projectile)
		}
	}
	s.lobby.Projectiles = projectiles
}

func (s *Server) applyCombatResolutionLocked(
	now time.Time,
	damage map[string]float64,
	sources map[string]combatSource,
	combatants map[string]struct{},
	knockbacks []crashPair,
) {
	preMass := make(map[string]float64, len(s.lobby.Players))
	lethal := make(map[string]bool, len(s.lobby.Players))
	creditedVictims := make(map[string][]string)

	for id, player := range s.lobby.Players {
		preMass[id] = player.Mass
		if !player.Alive {
			continue
		}

		if totalDamage := damage[id]; totalDamage > 0 {
			player.Health = math.Max(0, player.Health-totalDamage)
		}
		if player.Health <= 0 {
			lethal[id] = true
		}
	}

	for id := range combatants {
		if player := s.lobby.Players[id]; player != nil {
			player.LastCombatAt = now
		}
	}

	for victimID := range lethal {
		source, ok := sources[victimID]
		if !ok || source.killerID == "" || source.killerID == victimID {
			continue
		}
		creditedVictims[source.killerID] = append(creditedVictims[source.killerID], victimID)
	}

	for killerID, victims := range creditedVictims {
		killer := s.lobby.Players[killerID]
		if killer == nil {
			continue
		}
		killer.Score += len(victims)
		killer.Kills += len(victims)
		for _, victimID := range victims {
			s.setPlayerMassPreservingHealth(killer, killer.Mass+s.killMassTransfer(preMass[victimID]))
		}
	}

	for killerID, victims := range creditedVictims {
		killer := s.lobby.Players[killerID]
		if killer == nil || lethal[killerID] {
			continue
		}
		for range victims {
			killer.Health = math.Min(
				s.maxHealthForMass(killer.Mass),
				killer.Health+s.killHealAmount(killer.Mass),
			)
		}
	}

	for _, pair := range knockbacks {
		left := s.lobby.Players[pair.leftID]
		right := s.lobby.Players[pair.rightID]
		if left == nil || right == nil || lethal[left.ID] || lethal[right.ID] {
			continue
		}
		s.applyCrashKnockback(left, right)
	}

	for victimID := range lethal {
		source := sources[victimID]
		killer := s.lobby.Players[source.killerID]
		if source.kind == combatSourceCrash {
			CrashLethalOutcomes.Inc()
		}
		s.killPlayerLocked(s.lobby.Players[victimID], killer, source.reason, now)
	}
}

func (s *Server) applyCrashKnockback(left, right *Player) {
	axisX, axisY := s.separationAxis(left, right)
	overlap := s.radiusForMass(left.Mass) + s.radiusForMass(right.Mass) - math.Hypot(left.X-right.X, left.Y-right.Y)
	positionPush := math.Min(math.Max(overlap/2+1, 0), 20)

	left.X += axisX * positionPush
	left.Y += axisY * positionPush
	right.X -= axisX * positionPush
	right.Y -= axisY * positionPush

	left.VX += axisX * s.cfg.CrashKnockbackImpulse
	left.VY += axisY * s.cfg.CrashKnockbackImpulse
	right.VX -= axisX * s.cfg.CrashKnockbackImpulse
	right.VY -= axisY * s.cfg.CrashKnockbackImpulse

	s.clampPlayerToWorldLocked(left)
	s.clampPlayerToWorldLocked(right)
}

func (s *Server) separationAxis(left, right *Player) (float64, float64) {
	dx := left.X - right.X
	dy := left.Y - right.Y
	distance := math.Hypot(dx, dy)
	if distance >= 0.001 {
		return dx / distance, dy / distance
	}

	velocityX := left.VX - right.VX
	velocityY := left.VY - right.VY
	velocityMagnitude := math.Hypot(velocityX, velocityY)
	if velocityMagnitude >= 0.001 {
		return velocityX / velocityMagnitude, velocityY / velocityMagnitude
	}

	if left.ID < right.ID {
		return 1, 0
	}
	return -1, 0
}

func (s *Server) handleRespawnsLocked(now time.Time) {
	for _, player := range s.lobby.Players {
		if player.Alive || player.RespawnAt.IsZero() || now.Before(player.RespawnAt) {
			continue
		}

		player.Alive = true
		player.Mass = s.respawnMass(player.PreDeathMass)
		player.Health = s.maxHealthForMass(player.Mass)
		player.VX = 0
		player.VY = 0
		player.KilledBy = ""
		player.DeathReason = ""
		player.RespawnAt = time.Time{}
		player.SpawnInvulnerableUntil = now.Add(s.cfg.SpawnInvulnerabilityDuration)

		usedFallback := s.spawnPlayerAtRandomPositionLocked(player)
		player.PendingSpawnSeparation = usedFallback
	}
}

func (s *Server) resolveExpiredSpawnSeparationsLocked(now time.Time) {
	for _, player := range s.lobby.Players {
		if !player.Alive || !player.PendingSpawnSeparation || s.isInvulnerable(player, now) {
			continue
		}
		s.runSpawnSeparationLocked(player)
		player.PendingSpawnSeparation = false
	}
}

func (s *Server) runSpawnSeparationLocked(player *Player) {
	for step := 0; step < spawnSeparationSteps; step++ {
		other, overlap := s.nearestOverlappingPlayerLocked(player)
		if other == nil || overlap <= 0 {
			return
		}

		axisX, axisY := s.separationAxis(player, other)
		pushDistance := math.Min(overlap+1, spawnSeparationMaxPush)
		player.X += axisX * pushDistance
		player.Y += axisY * pushDistance
		s.clampPlayerToWorldLocked(player)
	}
}

func (s *Server) nearestOverlappingPlayerLocked(player *Player) (*Player, float64) {
	var nearest *Player
	bestDistance := math.MaxFloat64
	bestOverlap := 0.0

	for _, candidate := range s.lobby.Players {
		if candidate.ID == player.ID || !candidate.Alive {
			continue
		}

		distance := math.Hypot(player.X-candidate.X, player.Y-candidate.Y)
		overlap := s.radiusForMass(player.Mass) + s.radiusForMass(candidate.Mass) - distance
		if overlap <= 0 {
			continue
		}
		if distance < bestDistance {
			bestDistance = distance
			bestOverlap = overlap
			nearest = candidate
		}
	}

	return nearest, bestOverlap
}

func (s *Server) spawnPlayerAtRandomPositionLocked(player *Player) bool {
	radius := s.radiusForMass(math.Max(player.Mass, s.cfg.StartingMass))
	x, y, usedFallback := s.findClearSpawnPositionLocked(player.ID, radius)
	player.X = x
	player.Y = y
	return usedFallback
}

func (s *Server) findClearSpawnPositionLocked(playerID string, radius float64) (float64, float64, bool) {
	bestX := radius + 20
	bestY := radius + 20
	bestNearest := -1.0

	for attempt := 0; attempt < s.cfg.SpawnClearanceAttempts; attempt++ {
		x := s.randFloat(radius+20, s.cfg.WorldWidth-radius-20)
		y := s.randFloat(radius+20, s.cfg.WorldHeight-radius-20)
		clear, nearest := s.spawnPositionClearanceLocked(playerID, x, y, radius)
		if clear {
			return x, y, false
		}
		if nearest > bestNearest {
			bestNearest = nearest
			bestX = x
			bestY = y
		}
	}

	return bestX, bestY, true
}

func (s *Server) spawnPositionClearanceLocked(playerID string, x, y, radius float64) (bool, float64) {
	nearest := math.MaxFloat64
	clear := true
	for _, candidate := range s.lobby.Players {
		if candidate.ID == playerID || !candidate.Alive {
			continue
		}
		distance := math.Hypot(candidate.X-x, candidate.Y-y)
		nearest = math.Min(nearest, distance)
		if distance < radius+s.radiusForMass(candidate.Mass) {
			clear = false
		}
	}
	if nearest == math.MaxFloat64 {
		nearest = s.cfg.WorldWidth + s.cfg.WorldHeight
	}
	return clear, nearest
}

func (s *Server) pruneCrashPairsLocked(now time.Time) {
	for key, seenAt := range s.lobby.CrashPairs {
		if now.Sub(seenAt) >= s.cfg.CrashPairCooldown {
			delete(s.lobby.CrashPairs, key)
		}
	}
}

func (s *Server) isCrashPairOnCooldown(leftID, rightID string, now time.Time) bool {
	lastSeen, ok := s.lobby.CrashPairs[stablePairKey(leftID, rightID)]
	if !ok {
		return false
	}
	return now.Sub(lastSeen) < s.cfg.CrashPairCooldown
}

func (s *Server) applyPassiveHealingLocked(now time.Time) {
	dtSeconds := 1.0 / float64(s.cfg.TickRate)
	healDelta := s.passiveHealDelta(dtSeconds)
	for _, player := range s.lobby.Players {
		if !player.Alive {
			continue
		}
		if !player.LastCombatAt.IsZero() && now.Sub(player.LastCombatAt) < s.cfg.PassiveHealCombatDelay {
			continue
		}
		player.Health = math.Min(s.maxHealthForMass(player.Mass), player.Health+healDelta)
	}
}
