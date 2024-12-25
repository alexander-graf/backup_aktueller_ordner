package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	MaxBackups int
	Debug      bool
	Excludes   []string
	BackupDir  string
	TimeFormat string
}

var defaultConfig = Config{
	MaxBackups: 10,
	Debug:      true,
	TimeFormat: "02012006_150405",
	Excludes: []string{
		// Entwicklungsumgebungen
		".idea",
		".vscode",
		".eclipse",
		".settings",

		// Version Control
		".git",
		".gitignore",
		".svn",
		".hg",

		// Temporäre Dateien
		"*.tmp",
		"*.temp",
		"*.swp",
		"*~",

		// Logs
		"*.log",
		"logs/",

		// Python
		"venv",
		".venv",
		"__pycache__",
		"*.pyc",
		"*.pyo",
		"*.pyd",
		".Python",
		"pip-log.txt",
		".tox",
		".coverage",
		".pytest_cache",

		// Node.js
		"node_modules",
		"npm-debug.log",
		"yarn-debug.log",
		"yarn-error.log",
		".npm",

		// Rust
		"target/",
		"Cargo.lock",
		"**/*.rs.bk",

		// Go
		"bin/",
		"pkg/",
		"*.exe",
		"*.test",
		"*.prof",

		// Zig
		"zig-cache/",
		"zig-out/",

		// Build Verzeichnisse
		"build/",
		"dist/",
		"out/",

		// Konfigurationsdateien
		".env",
		".env.local",
		".env.*",
		"config.local.*",

		// Betriebssystem
		".DS_Store",
		"Thumbs.db",
		"desktop.ini",

		// IDEs und Editoren
		"*.sublime-workspace",
		"*.sublime-project",
		".atom/",
		".project",
		"*.iml",

		// Kompilierte Dateien
		"*.o",
		"*.a",
		"*.so",
		"*.dylib",
		"*.dll",
		"*.class",
	},
}

var currentBackup string

type LogLevel int

const (
	LogError LogLevel = iota
	LogWarning
	LogInfo
	LogDebug
)

func logMessage(level LogLevel, format string, a ...interface{}) {
	prefix := ""
	switch level {
	case LogError:
		prefix = "FEHLER: "
	case LogWarning:
		prefix = "WARNUNG: "
	case LogInfo:
		prefix = "INFO: "
	case LogDebug:
		if !defaultConfig.Debug {
			return
		}
		prefix = "DEBUG: "
	}
	fmt.Printf(prefix+format+"\n", a...)
}

func handleError(message string, err error, cleanup func()) {
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
		os.Exit(1)
	}
}

func checkTarAvailable() error {
	_, err := exec.LookPath("tar")
	if err != nil {
		return fmt.Errorf("tar ist nicht installiert: %v", err)
	}
	return nil
}

func checkPermissions(dir string) error {
	// Prüfe Lese- und Schreibrechte
	tempFile := filepath.Join(dir, ".backup_test")
	err := os.WriteFile(tempFile, []byte("test"), 0644)
	if err != nil {
		return fmt.Errorf("keine Schreibrechte in %s: %v", dir, err)
	}
	defer os.Remove(tempFile)

	_, err = os.ReadFile(tempFile)
	if err != nil {
		return fmt.Errorf("keine Leserechte in %s: %v", dir, err)
	}
	return nil
}

func isValidBackupName(name string) bool {
	// Prüfe auf ungültige Zeichen im Dateinamen
	return !strings.ContainsAny(name, "\\/:*?\"<>|")
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &defaultConfig, nil
		}
		return nil, err
	}
	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("fehler beim Lesen der Konfiguration: %v", err)
	}
	return &config, nil
}

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nProgramm wird beendet...")
		// Cleanup falls nötig
		if currentBackup != "" {
			os.Remove(currentBackup)
		}
		os.Exit(1)
	}()

	err := checkTarAvailable()
	handleError("fehler: tar wird benötigt", err, nil)

	// Lade Konfiguration aus config.json im aktuellen Verzeichnis
	config, err := loadConfig("config.json")
	if err != nil {
		logMessage(LogWarning, "Konnte Konfigurationsdatei nicht laden: %v\nVerwende Standardeinstellungen", err)
		config = &defaultConfig
	}

	// Absolute Pfade ermitteln
	sourceDir, err := os.Getwd()
	handleError("fehler beim Ermitteln des aktuellen Verzeichnisses", err, nil)
	logMessage(LogInfo, "Quellverzeichnis: %s", sourceDir)

	projectName := filepath.Base(sourceDir)
	if config.BackupDir == "" {
		config.BackupDir = filepath.Join(filepath.Dir(sourceDir), "Backup")
	}
	logMessage(LogInfo, "Projektname: %s", projectName)
	logMessage(LogInfo, "Backup-Verzeichnis: %s", config.BackupDir)

	// Backup-Verzeichnis erstellen
	if err := os.MkdirAll(config.BackupDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "fehler beim Erstellen des Backup-Verzeichnisses: %v\n", err)
		os.Exit(1)
	}
	logMessage(LogInfo, "Backup-Verzeichnis erstellt oder existiert bereits")

	// Alte Backups aufräumen
	err = cleanupOldBackups(config.BackupDir, projectName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fehler beim Aufräumen alter Backups: %v\n", err)
		os.Exit(1)
	}

	// Zeitstempel für Backup-Datei
	timestamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(config.BackupDir, fmt.Sprintf("%s_backup_%s.tar.gz", projectName, timestamp))
	logMessage(LogInfo, "Backup-Datei: %s", backupFile)

	// Speicherplatz prüfen
	err = checkDiskSpace(sourceDir, config.BackupDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fehler beim Prüfen des Speicherplatzes: %v\n", err)
		os.Exit(1)
	}
	logMessage(LogInfo, "Ausreichend Speicherplatz verfügbar")

	// Vor der Backup-Erstellung:
	if !isValidBackupName(projectName) {
		handleError("fehler: ungültiger Projektname",
			fmt.Errorf("name enthält ungültige Zeichen: %s", projectName), nil)
	}

	// Backup erstellen
	err = createBackup(sourceDir, backupFile)
	handleError("fehler beim Erstellen des Backups", err, func() {
		os.Remove(backupFile)
	})

	// Backup-Größe ermitteln
	fileInfo, err := os.Stat(backupFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fehler beim Ermitteln der Backup-Größe: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Backup erstellt: %s\n", backupFile)
	fmt.Printf("  Größe: %s\n", formatSize(fileInfo.Size()))

	// Aktuelle Backups anzeigen
	err = listBackups(config.BackupDir, projectName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fehler beim Auflisten der Backups: %v\n", err)
		os.Exit(1)
	}

	// Backup-Integrität zum Schluss prüfen
	fmt.Printf("\nVerifiziere Backup-Integrität...\n")
	err = verifyBackup(backupFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fehler bei der Backup-Verifizierung: %v\n", err)
		os.Remove(backupFile)
		os.Exit(1)
	}
	fmt.Printf("+ Backup-Integrität bestätigt\n")

	err = checkPermissions(config.BackupDir)
	handleError("fehler: unzureichende Berechtigungen", err, nil)
}

func cleanupOldBackups(backupDir, projectName string) error {
	logMessage(LogInfo, "Suche nach alten Backups...")
	pattern := filepath.Join(backupDir, fmt.Sprintf("%s_backup_*.tar.gz", projectName))
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	type BackupFile struct {
		path    string
		modTime time.Time
	}

	var backups []BackupFile
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			logMessage(LogWarning, "Warnung: Kann Status von %s nicht lesen: %v", file, err)
			continue
		}
		backups = append(backups, BackupFile{file, info.ModTime()})
	}

	// Sortiere nach Datum (neueste zuerst)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.After(backups[j].modTime)
	})

	if len(backups) > defaultConfig.MaxBackups {
		logMessage(LogInfo, "Maximale Backup-Anzahl erreicht, lösche %d alte Backups", len(backups)-defaultConfig.MaxBackups)
		for i := defaultConfig.MaxBackups; i < len(backups); i++ {
			logMessage(LogInfo, "Lösche: %s", backups[i].path)
			if err := os.Remove(backups[i].path); err != nil {
				return fmt.Errorf("fehler beim Löschen von %s: %v", backups[i].path, err)
			}
		}
	}
	return nil
}

func checkDiskSpace(sourceDir, backupDir string) error {
	logMessage(LogInfo, "Prüfe verfügbaren Speicherplatz...")

	// Quellgröße ermitteln
	var sourceSize int64
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			sourceSize += info.Size()
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("fehler beim Ermitteln der Quellgröße: %v", err)
	}

	if sourceSize == 0 {
		return fmt.Errorf("quellverzeichnis scheint leer zu sein")
	}

	// Verfügbaren Speicherplatz ermitteln
	var stat syscall.Statfs_t
	err = syscall.Statfs(backupDir, &stat)
	if err != nil {
		return fmt.Errorf("fehler beim Ermitteln des verfügbaren Speicherplatzes: %v", err)
	}

	available := stat.Bavail * uint64(stat.Bsize)
	required := uint64(float64(sourceSize) * 1.1) // 10% extra für Komprimierung

	// Mindestens 50MB oder 10% der Quellgröße frei lassen
	minSpace := uint64(50 * 1024 * 1024)
	if required < minSpace {
		required = minSpace
	}

	if available < required {
		return fmt.Errorf("nicht genügend Speicherplatz. benötigt: %s, verfügbar: %s",
			formatSize(int64(required)),
			formatSize(int64(available)))
	}

	logMessage(LogInfo, "Quellgröße: %s", formatSize(sourceSize))
	logMessage(LogInfo, "Verfügbarer Speicherplatz: %s", formatSize(int64(available)))
	return nil
}

func createBackup(sourceDir, backupFile string) error {
	logMessage(LogInfo, "Erstelle Backup...")
	args := []string{"-czf", backupFile, "-C", sourceDir}

	for _, exclude := range defaultConfig.Excludes {
		args = append(args, "--exclude="+exclude)
	}
	args = append(args, ".")

	cmd := exec.Command("tar", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Erstelle Backup von %s\n", sourceDir)
	fmt.Printf("Ausgeschlossene Dateien/Ordner: %s\n", strings.Join(defaultConfig.Excludes, ", "))

	startTime := time.Now()
	err := cmd.Run()
	if err != nil {
		return err
	}

	duration := time.Since(startTime)
	fmt.Printf("Backup-Erstellung abgeschlossen in %v\n", duration.Round(time.Second).String())
	return nil
}

func verifyBackup(backupFile string) error {
	logMessage(LogInfo, "Verifiziere Backup...")
	cmd := exec.Command("tar", "-tzf", backupFile)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func listBackups(backupDir, projectName string) error {
	logMessage(LogInfo, "Liste aktuelle Backups auf...")
	pattern := filepath.Join(backupDir, fmt.Sprintf("%s_backup_*.tar.gz", projectName))
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	var totalSize int64
	validFiles := 0
	fmt.Println("\nAktuelle Backups:")
	for _, file := range files {
		fileInfo, err := os.Stat(file)
		if err != nil {
			continue
		}
		totalSize += fileInfo.Size()
		validFiles++
		fmt.Printf("%s vom %s (%s)\n",
			filepath.Base(file),
			formatDateTime(fileInfo.ModTime()),
			formatSize(fileInfo.Size()))
	}

	if validFiles > 0 {
		fmt.Printf("\nGesamtanzahl Backups: %d", validFiles)
		fmt.Printf("\nGesamtgröße: %s\n", formatSize(totalSize))
	}
	return nil
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatDateTime(t time.Time) string {
	// Deutsches Format für die Anzeige: TT.MM.YYYY HH:MM:SS
	return t.Format("02.01.2006 15:04:05")
}
