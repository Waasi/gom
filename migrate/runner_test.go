package migrate

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/waasi/gom/internal/testmongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testmongo.Stop()
	os.Exit(code)
}

// marker returns a migration that inserts/removes a doc in "marks", so tests
// can see exactly which Up and Down functions ran.
func marker(v int64, name string) Migration {
	return Migration{
		Version: v, Name: name,
		Up: func(ctx context.Context, db *mongo.Database) error {
			_, err := db.Collection("marks").InsertOne(ctx, bson.M{"_id": v})
			return err
		},
		Down: func(ctx context.Context, db *mongo.Database) error {
			_, err := db.Collection("marks").DeleteOne(ctx, bson.M{"_id": v})
			return err
		},
	}
}

func marks(t *testing.T, db *mongo.Database) int64 {
	t.Helper()
	n, err := db.Collection("marks").CountDocuments(context.Background(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUpDownStatus(t *testing.T) {
	db := testmongo.DB(t)
	ctx := context.Background()
	// Deliberately out of order: the runner must sort by version.
	r := &Runner{DB: db, Migrations: []Migration{marker(3, "c"), marker(1, "a"), marker(2, "b")}}

	n, err := r.Up(ctx)
	if err != nil || n != 3 {
		t.Fatalf("Up = %d, %v; want 3, nil", n, err)
	}
	if n, _ := r.Up(ctx); n != 0 {
		t.Fatalf("second Up ran %d migrations; want 0", n)
	}
	if got := marks(t, db); got != 3 {
		t.Fatalf("marks = %d, want 3", got)
	}

	n, err = r.Down(ctx, 2)
	if err != nil || n != 2 {
		t.Fatalf("Down(2) = %d, %v; want 2, nil", n, err)
	}
	rows, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, false}
	for i, row := range rows {
		if row.Version != int64(i+1) || row.Applied != want[i] {
			t.Errorf("status[%d] = v%d applied=%v; want v%d applied=%v", i, row.Version, row.Applied, i+1, want[i])
		}
	}
	if got := marks(t, db); got != 1 {
		t.Fatalf("marks after Down(2) = %d, want 1", got)
	}
}

func TestUpStopsAtFailureAndRetries(t *testing.T) {
	db := testmongo.DB(t)
	ctx := context.Background()
	boom := errors.New("boom")
	fail := true
	bad := Migration{Version: 2, Name: "bad", Up: func(ctx context.Context, db *mongo.Database) error {
		if fail {
			return boom
		}
		return nil
	}}
	r := &Runner{DB: db, Migrations: []Migration{marker(1, "a"), bad, marker(3, "c")}}

	n, err := r.Up(ctx)
	if !errors.Is(err, boom) || n != 1 {
		t.Fatalf("Up = %d, %v; want 1, boom", n, err)
	}
	if got := marks(t, db); got != 1 {
		t.Fatalf("migration after the failure ran: marks = %d", got)
	}

	fail = false
	if n, err := r.Up(ctx); err != nil || n != 2 {
		t.Fatalf("retry Up = %d, %v; want 2, nil", n, err)
	}
}

func TestDownRefusesIrreversible(t *testing.T) {
	db := testmongo.DB(t)
	ctx := context.Background()
	oneWay := marker(2, "one_way")
	oneWay.Down = nil
	r := &Runner{DB: db, Migrations: []Migration{marker(1, "a"), oneWay}}
	if _, err := r.Up(ctx); err != nil {
		t.Fatal(err)
	}
	n, err := r.Down(ctx, 2)
	if !errors.Is(err, ErrIrreversible) || n != 0 {
		t.Fatalf("Down = %d, %v; want 0, ErrIrreversible", n, err)
	}
}

func TestStatusReportsMissingFile(t *testing.T) {
	db := testmongo.DB(t)
	ctx := context.Background()
	if _, err := (&Runner{DB: db, Migrations: []Migration{marker(1, "gone")}}).Up(ctx); err != nil {
		t.Fatal(err)
	}
	r := &Runner{DB: db, Migrations: []Migration{}}
	rows, err := r.Status(ctx)
	if err != nil || len(rows) != 1 || !rows[0].Applied || rows[0].Up != nil || rows[0].Name != "gone" {
		t.Fatalf("Status = %+v, %v; want one applied row with no code", rows, err)
	}
	if _, err := r.Down(ctx, 1); err == nil {
		t.Fatal("Down rolled back a migration whose code is missing")
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate version did not panic")
		}
		delete(registry, 99)
	}()
	Register(marker(99, "x"))
	Register(marker(99, "y"))
}
