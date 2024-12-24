# Backup-Skript für Entwicklungsprojekte

Ein einfaches Bash-Skript zum automatischen Backup von Entwicklungsprojekten.

## Features

- Erstellt komprimierte Backups (.tar.gz) des aktuellen Verzeichnisses
- Schließt typische Entwicklungsordner automatisch aus (node_modules, venv, .git, etc.)
- Speichert Backups mit Zeitstempel im übergeordneten "Backup"-Verzeichnis
- Begrenzt die Anzahl der Backups pro Projekt (standardmäßig 10)
- Prüft verfügbaren Speicherplatz vor dem Backup
- Zeigt Fortschritt und Backup-Größe an

## Installation

1. Laden Sie die Datei `backup_dieser_ordner.sh` herunter
2. Machen Sie das Skript ausführbar:
   ```bash
   chmod +x backup_dieser_ordner.sh
   ```

## Verwendung

Führen Sie das Skript im Verzeichnis aus, das Sie sichern möchten:



