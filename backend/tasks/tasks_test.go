package tasks

import "testing"

// Running rows lead, oldest first, so a long pass stays where the user found
// it; finished rows follow by most recent update.
func TestSortRunningFirstThenRecent(t *testing.T) {
	list := []Task{
		{ID: "done-old", State: StateDone, UpdatedAt: 10},
		{ID: "run-new", State: StateRunning, StartedAt: 200},
		{ID: "done-new", State: StateDone, UpdatedAt: 30},
		{ID: "run-old", State: StateRunning, StartedAt: 100},
	}
	Sort(list)

	want := []string{"run-old", "run-new", "done-new", "done-old"}
	for i, id := range want {
		if list[i].ID != id {
			t.Fatalf("row %d = %q, want %q (list = %+v)", i, list[i].ID, id, list)
		}
	}
}
