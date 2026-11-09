// Package testmongo gives tests a throwaway MongoDB database, or skips them
// when no server is available.
//
//   - GOM_TEST_MONGO_URI: use an existing server.
//   - GOM_TEST_MONGOD: path to a mongod binary to start on a free port with a
//     temporary data directory (falls back to mongod on $PATH).
package testmongo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var (
	once    sync.Once
	uri     string
	why     error
	proc    *exec.Cmd
	dataDir string
)

func start() {
	if u := os.Getenv("GOM_TEST_MONGO_URI"); u != "" {
		uri = u
		return
	}
	bin := os.Getenv("GOM_TEST_MONGOD")
	if bin == "" {
		var err error
		if bin, err = exec.LookPath("mongod"); err != nil {
			why = fmt.Errorf("set GOM_TEST_MONGO_URI or GOM_TEST_MONGOD (no mongod on PATH)")
			return
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		why = err
		return
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	if dataDir, err = os.MkdirTemp("", "gom-mongod-"); err != nil {
		why = err
		return
	}
	proc = exec.Command(bin, "--dbpath", dataDir, "--port", strconv.Itoa(port), "--bind_ip", "127.0.0.1", "--quiet")
	if err := proc.Start(); err != nil {
		why = fmt.Errorf("start %s: %w", bin, err)
		return
	}
	u := fmt.Sprintf("mongodb://127.0.0.1:%d", port)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if ping(u) == nil {
			uri = u
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	why = fmt.Errorf("mongod did not accept connections within 30s")
	Stop()
}

func ping(u string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, err := mongo.Connect(ctx, options.Client().ApplyURI(u).SetServerSelectionTimeout(time.Second))
	if err != nil {
		return err
	}
	defer c.Disconnect(context.Background())
	return c.Ping(ctx, readpref.Primary())
}

// DB returns a fresh, uniquely named database, dropped when the test ends.
func DB(t *testing.T) *mongo.Database {
	t.Helper()
	once.Do(start)
	if uri == "" {
		t.Skipf("mongodb unavailable: %v", why)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	db := client.Database("gomtest_" + hex.EncodeToString(b))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	return db
}

// Stop shuts down a mongod this package started; call it from TestMain.
func Stop() {
	if proc != nil && proc.Process != nil {
		_ = proc.Process.Kill()
		_ = proc.Wait()
		proc = nil
	}
	if dataDir != "" {
		_ = os.RemoveAll(dataDir)
		dataDir = ""
	}
}
