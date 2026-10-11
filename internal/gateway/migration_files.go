package gateway

// MigrationFiles includes external Gateway template backups, not only DB rows.
func MigrationFiles() []string { return []string{backupPath, singBoxBackupPath} }
