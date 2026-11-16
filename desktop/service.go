package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/schollz/croc/v11/src/codephrase"
	"github.com/schollz/croc/v11/src/croc"
	"github.com/schollz/croc/v11/src/models"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type DesktopService struct {
	mu            sync.RWMutex
	app           *application.App
	manager       *TransferManager
	receiveDir    string
	selectedFiles []string
	relayAddress  string
	relayPassword string
}

type DesktopSettings struct {
	ReceiveFolder string `json:"receiveFolder"`
	RelayAddress  string `json:"relayAddress"`
	RelayPassword string `json:"relayPassword"`
}

func NewDesktopService() *DesktopService {
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = "."
	}
	dir = filepath.Join(dir, "Downloads")
	return &DesktopService{receiveDir: dir, relayAddress: models.DEFAULT_RELAY, relayPassword: models.DEFAULT_PASSPHRASE}
}

func (s *DesktopService) attach(app *application.App) {
	s.app = app
	s.manager = NewTransferManager(func(event TransferEvent) { app.Event.Emit("transfer-update", event) })
}

func (s *DesktopService) SelectFiles() ([]string, error) {
	paths, err := s.app.Dialog.OpenFile().SetTitle("Select files to send").CanChooseFiles(true).CanChooseDirectories(false).PromptForMultipleSelection()
	if err != nil {
		return nil, err
	}
	valid, err := validateSourcePaths(paths)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.selectedFiles = valid
	s.mu.Unlock()
	return append([]string(nil), valid...), nil
}

func (s *DesktopService) SetDroppedFiles(paths []string) ([]string, error) {
	valid, err := validateSourcePaths(paths)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.selectedFiles = valid
	s.mu.Unlock()
	return append([]string(nil), valid...), nil
}

func (s *DesktopService) SelectReceiveFolder() (string, error) {
	path, err := s.app.Dialog.OpenFile().SetTitle("Choose receive folder").CanChooseDirectories(true).CanChooseFiles(false).PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if path == "" {
		return s.ReceiveFolder(), nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("receive destination must be a directory")
	}
	s.mu.Lock()
	s.receiveDir = absolute
	s.mu.Unlock()
	return absolute, nil
}

func (s *DesktopService) ReceiveFolder() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.receiveDir
}

func (s *DesktopService) Settings() DesktopSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return DesktopSettings{ReceiveFolder: s.receiveDir, RelayAddress: s.relayAddress, RelayPassword: s.relayPassword}
}

func (s *DesktopService) UpdateRelay(address, password string) error {
	address = strings.TrimSpace(address)
	password = strings.TrimSpace(password)
	if address == "" {
		return errors.New("relay address is required")
	}
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("relay must be host:port: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("relay port must be between 1 and 65535")
	}
	if password == "" {
		return errors.New("relay password is required")
	}
	s.mu.Lock()
	s.relayAddress, s.relayPassword = address, password
	s.mu.Unlock()
	return nil
}

func (s *DesktopService) StartSend(paths []string) (TransferStart, error) {
	s.mu.RLock()
	if len(paths) == 0 {
		paths = append([]string(nil), s.selectedFiles...)
	}
	options := crocOptions(s.relayAddress, s.relayPassword)
	s.mu.RUnlock()
	result, err := s.manager.StartSend(paths, options)
	if err == nil {
		s.mu.Lock()
		s.selectedFiles = nil
		s.mu.Unlock()
	}
	return result, err
}

func (s *DesktopService) StartReceive(secret string) (TransferStart, error) {
	secret = strings.TrimSpace(secret)
	if _, err := codephrase.Parse(secret); err != nil {
		return TransferStart{}, fmt.Errorf("invalid Croc share code: %w", err)
	}
	s.mu.RLock()
	receiveDir := s.receiveDir
	options := crocOptions(s.relayAddress, s.relayPassword)
	s.mu.RUnlock()
	if err := os.MkdirAll(receiveDir, 0o700); err != nil {
		return TransferStart{}, fmt.Errorf("prepare receive folder: %w", err)
	}
	options.ReceiveDir = receiveDir
	options.Rename = true // preserve collisions without prompting or overwriting
	options.Overwrite = false
	options.NoPrompt = true // the user explicitly started this receive session
	return s.manager.StartReceive(secret, options)
}

func (s *DesktopService) CancelTransfer() error          { return s.manager.Cancel() }
func (s *DesktopService) CurrentTransfer() TransferEvent { return s.manager.Snapshot() }

func crocOptions(relay, password string) croc.Options {
	if relay == "" {
		relay = models.DEFAULT_RELAY
	}
	if password == "" {
		password = models.DEFAULT_PASSPHRASE
	}
	ports := make([]string, 9)
	for i := range ports {
		ports[i] = fmt.Sprint(9009 + i)
	}
	return croc.Options{
		RelayAddress:     relay,
		RelayPassword:    password,
		RelayPorts:       ports,
		Curve:            "p256",
		HashAlgorithm:    "xxhash",
		Transport:        croc.TransportAuto,
		NoPrompt:         true,
		DisableClipboard: true,
		SuppressOutput:   true,
	}
}

func validateSourcePaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, errors.New("select at least one file")
	}
	valid := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, value := range paths {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("file path cannot be empty")
		}
		path, err := filepath.Abs(value)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("cannot access %q: %w", path, err)
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil, fmt.Errorf("unsupported file type: %s", path)
		}
		key := strings.ToLower(filepath.Clean(path))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		valid = append(valid, path)
	}
	if len(valid) == 0 {
		return nil, errors.New("select at least one file")
	}
	return valid, nil
}
