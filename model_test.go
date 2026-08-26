package main

import "testing"

func TestQuestCompletionUnlocksLayer(t *testing.T) {
	w := NewWorld()
	w.Player = w.Quests[0].Position
	if !w.OpenNearbyQuest(50) {
		t.Fatal("expected a nearby quest")
	}
	if w.Answer(2) {
		t.Fatal("wrong answer must not complete the quest")
	}
	if w.LayerUnlocked(LayerMovement) {
		t.Fatal("layer unlocked after a wrong answer")
	}
	if !w.Answer(w.Quests[0].Correct) {
		t.Fatal("correct answer should complete the quest")
	}
	if !w.LayerUnlocked(LayerMovement) {
		t.Fatal("movement layer was not unlocked")
	}
}

func TestCompletingAllQuestsShowsEnding(t *testing.T) {
	w := NewWorld()
	for i := range w.Quests {
		w.ActiveQuest = i
		if !w.Answer(w.Quests[i].Correct) {
			t.Fatalf("quest %d did not complete", i)
		}
	}
	if !w.ShowComplete {
		t.Fatal("completion screen was not enabled")
	}
	if got, want := w.CompletedCount(), len(w.Quests); got != want {
		t.Fatalf("completed %d quests, want %d", got, want)
	}
}

func TestResetRestoresInitialState(t *testing.T) {
	w := NewWorld()
	w.Quests[0].Completed = true
	w.Player = Point{X: 10, Y: 20}
	w.ShowComplete = true
	w.Reset()
	if w.CompletedCount() != 0 || w.ShowComplete {
		t.Fatal("reset did not clear progress")
	}
	if w.Player != (Point{X: 430, Y: 500}) {
		t.Fatalf("unexpected reset position: %#v", w.Player)
	}
}
