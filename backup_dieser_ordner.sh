#!/bin/bash
set -euo pipefail

# Kurzanleitung
# -------------
# Dieses Skript erstellt ein Backup des aktuellen Verzeichnisses.
# - Ausgeschlossen werden typische Entwicklungsordner (node_modules, venv, etc.)
# - Backups werden im übergeordneten Verzeichnis unter "Backup" gespeichert
# - Es werden maximal 10 Backups pro Projekt aufbewahrt (älteste werden gelöscht)
# 
# Verwendung:
# ./backup_dieser_ordner.sh
#
# Das Backup wird als .tar.gz Datei gespeichert mit Zeitstempel im Namen:
# projektname_backup_YYYYMMDD_HHMMSS.tar.gz


# Konfiguration
MAX_BACKUPS=10  # Maximale Anzahl der Backups pro Projekt

# Standard-Ausschlüsse für Entwicklungsprojekte
EXCLUDES=(
    "node_modules"
    "venv"
    ".git"
    "__pycache__"
    "*.log"
    "*.pyc"
    "build"
    "dist"
    ".env"
    ".idea"
    ".vscode"
)

# Absolute Pfade verwenden
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_NAME=$(basename "$SOURCE_DIR")
BACKUP_DIR="$(cd "$SOURCE_DIR/.." && pwd)/Backup"

# Prüfe Schreibrechte für Backup-Verzeichnis
if ! mkdir -p "$BACKUP_DIR" 2>/dev/null; then
    echo "Fehler: Keine Schreibrechte für $BACKUP_DIR" >&2
    exit 1
fi

# Lösche alte Backups wenn Maximum erreicht
cleanup_old_backups() {
    local count=$(ls -1 "$BACKUP_DIR/${PROJECT_NAME}_backup_"* 2>/dev/null | wc -l)
    if [ "$count" -ge "$MAX_BACKUPS" ]; then
        echo "Entferne alte Backups..."
        ls -1t "$BACKUP_DIR/${PROJECT_NAME}_backup_"* | tail -n +$((MAX_BACKUPS + 1)) | xargs rm -f
    fi
}

# Zeitstempel für den Dateinamen
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="$BACKUP_DIR/${PROJECT_NAME}_backup_$TIMESTAMP.tar.gz"

# Erstelle Exclude-Parameter für tar
EXCLUDE_OPTS=""
for item in "${EXCLUDES[@]}"; do
    EXCLUDE_OPTS="$EXCLUDE_OPTS --exclude=$item"
done

# Cleanup vor neuem Backup
cleanup_old_backups

# Prüfe ob genügend Speicherplatz verfügbar ist
SOURCE_SIZE=$(du -s "$SOURCE_DIR" | cut -f1)
BACKUP_SPACE=$(df -P "$BACKUP_DIR" | awk 'NR==2 {print $4}')

if [ "$SOURCE_SIZE" -gt "$BACKUP_SPACE" ]; then
    echo "Fehler: Nicht genügend Speicherplatz für das Backup" >&2
    exit 1
fi

# Erstelle das Backup mit Fortschrittsanzeige
echo "Erstelle Backup von $SOURCE_DIR..."
tar -czf "$BACKUP_FILE" -C "$SOURCE_DIR" $EXCLUDE_OPTS . || {
    echo "Fehler beim Erstellen des Backups" >&2
    rm -f "$BACKUP_FILE"  # Aufräumen bei Fehler
    exit 1
}

# Prüfe ob Backup erfolgreich erstellt wurde
if [ -f "$BACKUP_FILE" ]; then
    echo "✓ Backup erfolgreich erstellt: $BACKUP_FILE"
    echo "  Größe: $(du -h "$BACKUP_FILE" | cut -f1)"
    
    # Prüfe die Integrität des Backups
    echo "Prüfe Backup-Integrität..."
    if ! tar -tzf "$BACKUP_FILE" >/dev/null 2>&1; then
        echo "Fehler: Backup-Datei scheint beschädigt zu sein" >&2
        rm -f "$BACKUP_FILE"
        exit 1
    fi
    echo "✓ Backup-Integrität bestätigt"
else
    echo "Fehler: Backup-Datei wurde nicht erstellt" >&2
    exit 1
fi

# Zeige verbleibende Backups an
echo -e "\nAktuelle Backups:"
ls -lh "$BACKUP_DIR/${PROJECT_NAME}_backup_"* 2>/dev/null | awk '{print $9, "(" $5 ")"}'
