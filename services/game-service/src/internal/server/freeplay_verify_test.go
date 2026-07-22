package server

import (
	"testing"
	"time"

	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
)

// "Verify Freeplay continues beyond the normal final wave"
func TestFreeplayContinuesPastFinalWave(t *testing.T) {
	runtime := &runtimeSession{
		session: gamesession.State{Mode: gamesession.ModeFreePlay, Health: 1_000_000},
		path:         []gameobject.Position{{X: 0, Y: 0}, {X: 1, Y: 0}},
		nextWaveTick: 1,
		loopStarted:  true,
	}
	now := time.Now()
	highest := 0
	for tick := int64(0); tick < 200000; tick++ {
		runtime.session.Tick = tick
		advanceRuntimeTick(runtime, now)
		if runtime.session.Wave > highest {
			highest = runtime.session.Wave
		}
		if gameWon(*runtime) {
			t.Fatalf("gameWon returned true in freeplay mode at wave %d, must never win", runtime.session.Wave)
		}
		if highest > len(waveDefinitions()) {
			return // passed
		}
	}
	t.Fatalf("freeplay never reached here")
}

// "Verify Story Mode still ends normally"
func TestNormalModeStillWins(t *testing.T) {
	runtime := &runtimeSession{
		session:      gamesession.State{Mode: gamesession.ModeNormal, Health: 1_000_000},
		path:         []gameobject.Position{{X: 0, Y: 0}, {X: 1, Y: 0}},
		nextWaveTick: 1,
		loopStarted:  true,
	}
	now := time.Now()
	for tick := int64(0); tick < 200000; tick++ {
		runtime.session.Tick = tick
		advanceRuntimeTick(runtime, now)
		if gameWon(*runtime) {
			return // passed
		}
	}
	t.Fatalf("normal mode never reached victory (stuck at wave %d)", runtime.session.Wave)
}
