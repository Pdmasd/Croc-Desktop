package croc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestDesktopOptionsTransferThroughCrocCore(t *testing.T) {
	relay := startTestRelay(t, 1)
	root := t.TempDir()
	source := filepath.Join(root, "desktop-source.bin")
	receiveDir := filepath.Join(root, "selected-receive-folder")
	if err := os.Mkdir(receiveDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("Croc Desktop uses the existing Croc transfer core.\n"), 4096)
	if err := os.WriteFile(source, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	secret := "desktop-core-integration-secret"
	var callbackMu sync.Mutex
	var senderPhases []string
	var senderDone, senderTotal int64
	base := Options{
		SharedSecret:     secret,
		RelayAddress:     relay.address,
		RelayPorts:       append([]string(nil), relay.dataPorts...),
		RelayPassword:    "pass123",
		NoPrompt:         true,
		DisableLocal:     true,
		DisableClipboard: true,
		SuppressOutput:   true,
		Curve:            "siec",
		Transport:        TransportRelay,
		HashAlgorithm:    "xxhash",
		Overwrite:        true,
	}
	sendOptions := base
	sendOptions.IsSender = true
	sendOptions.OnPhase = func(phase string) { callbackMu.Lock(); senderPhases = append(senderPhases, phase); callbackMu.Unlock() }
	sendOptions.OnProgress = func(done, total int64) { callbackMu.Lock(); senderDone, senderTotal = done, total; callbackMu.Unlock() }
	receiveOptions := base
	receiveOptions.Transport = TransportAuto
	receiveOptions.ReceiveDir = receiveDir
	receiveOptions.Rename = true
	receiveOptions.Overwrite = true
	receiveOptions.OnPhase = func(string) {}
	receiveOptions.OnProgress = func(int64, int64) {}
	sender, err := NewCtx(t.Context(), sendOptions)
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := NewCtx(t.Context(), receiveOptions)
	if err != nil {
		t.Fatal(err)
	}
	files, empty, folders, err := GetFilesInfoContext(t.Context(), []string{source}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() { results <- sender.Send(files, empty, folders) }()
	time.Sleep(100 * time.Millisecond)
	go func() { results <- receiver.Receive() }()
	for completed := 0; completed < 2; completed++ {
		select {
		case transferErr := <-results:
			if transferErr != nil {
				t.Errorf("transfer client error: %v", transferErr)
			}
		case <-time.After(20 * time.Second):
			cancelClients(sender, receiver)
			t.Fatal("transfer timed out")
		}
	}
	received, err := os.ReadFile(filepath.Join(receiveDir, filepath.Base(source)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(received, payload) {
		t.Fatal("received bytes differ from source")
	}
	callbackMu.Lock()
	defer callbackMu.Unlock()
	if senderDone != int64(len(payload)) || senderTotal != int64(len(payload)) {
		t.Fatalf("reported byte counts = %d/%d; want %d/%d", senderDone, senderTotal, len(payload), len(payload))
	}
	if !slices.Contains(senderPhases, "Connecting") || !slices.Contains(senderPhases, "Authenticating") || !slices.Contains(senderPhases, "Transferring") {
		t.Fatalf("missing desktop lifecycle phases: %v", senderPhases)
	}
}

func TestDesktopCoreCancellationAndConnectionError(t *testing.T) {
	relay := startTestRelay(t, 1)
	options := Options{SharedSecret: "desktop-cancel-test", RelayAddress: relay.address, RelayPassword: "pass123", DisableLocal: true, NoPrompt: true, DisableClipboard: true, SuppressOutput: true, Curve: "p256", Transport: TransportAuto}
	ctx, cancel := context.WithCancel(t.Context())
	receiver, err := NewCtx(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- receiver.Receive() }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled receive returned nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("receive did not stop after context cancellation")
	}

	bad := Options{SharedSecret: "desktop-connection-error", RelayAddress: "127.0.0.1:1", RelayPassword: "pass123", DisableLocal: true, NoPrompt: true, DisableClipboard: true, SuppressOutput: true, Curve: "p256"}
	client, err := NewCtx(t.Context(), bad)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Receive()
	if err == nil {
		t.Fatal("unreachable relay did not return an error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("connect")) {
		t.Fatalf("connection error was not descriptive: %v", err)
	}
}
