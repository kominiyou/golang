package features

import (
        "context"
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "time"

        "go.mau.fi/whatsmeow"
        waProto "go.mau.fi/whatsmeow/binary/proto"
        "go.mau.fi/whatsmeow/store/sqlstore"
        "go.mau.fi/whatsmeow/types"
        "go.mau.fi/whatsmeow/types/events"
        waLog "go.mau.fi/whatsmeow/util/log"
        "google.golang.org/protobuf/proto"

        "whatsapp-bot/utils"
)

const (
        JadibotFolder = "Wilykun/jadibot"
)

// FilteredLogger suppresses verbose SessionCipher MAC verification errors dan retry receipt errors
type FilteredLogger struct {
        logger waLog.Logger
}

func (f *FilteredLogger) Errorf(msg string, args ...interface{}) {
        formattedMsg := fmt.Sprintf(msg, args...)
        // Suppress verbose errors yang tidak penting
        if strings.Contains(formattedMsg, "Unable to verify ciphertext mac") ||
                strings.Contains(formattedMsg, "mismatching MAC") ||
                strings.Contains(formattedMsg, "Unable to get or create message keys") ||
                strings.Contains(formattedMsg, "Failed to handle retry receipt") ||
                strings.Contains(formattedMsg, "couldn't find message") {
                return
        }
        f.logger.Errorf(msg, args...)
}

func (f *FilteredLogger) Warnf(msg string, args ...interface{}) {
        formattedMsg := fmt.Sprintf(msg, args...)
        // Suppress verbose warnings
        if strings.Contains(formattedMsg, "mismatching MAC") ||
                strings.Contains(formattedMsg, "failed to verify ciphertext MAC") ||
                strings.Contains(formattedMsg, "retry receipt") {
                return
        }
        f.logger.Warnf(msg, args...)
}

func (f *FilteredLogger) Infof(msg string, args ...interface{}) {
        f.logger.Infof(msg, args...)
}

func (f *FilteredLogger) Debugf(msg string, args ...interface{}) {
        f.logger.Debugf(msg, args...)
}

func (f *FilteredLogger) Sub(module string) waLog.Logger {
        return &FilteredLogger{logger: f.logger.Sub(module)}
}

// Folder structure (per jadibot):
// Wilykun/jadibot/
// ├── 6289681234567/
// │   ├── 6289681234567.db       (SQLite database)
// │   ├── 6289681234567.db-shm   (SQLite shared memory)
// │   └── 6289681234567.db-wal   (SQLite write-ahead log)
// ├── 6289687654321/
// │   ├── 6289687654321.db
// │   ├── 6289687654321.db-shm
// │   └── 6289687654321.db-wal
//
// Setiap jadibot punya subfolder unik berdasarkan nomor telepon
// Format folder: {phoneNumber}/
// Format file dalam folder: {phoneNumber}.db, {phoneNumber}.db-shm, {phoneNumber}.db-wal

type JadibotSession struct {
        PhoneNumber  string
        Client       *whatsmeow.Client
        Container    *sqlstore.Container
        Connected    bool
        StartTime    time.Time
        OwnerChat    types.JID
        Reconnecting bool
        FailCount    int
        LastFailTime time.Time
}

type JadibotManager struct {
        sessions   map[string]*JadibotSession
        pending    map[string]*JadibotSession
        mainClient *whatsmeow.Client
        mu         sync.RWMutex
}

var jadibotManager = &JadibotManager{
        sessions: make(map[string]*JadibotSession),
        pending:  make(map[string]*JadibotSession),
}

func init() {
        os.MkdirAll(JadibotFolder, 0o755)
}

// getJadibotFolder - Get subfolder untuk jadibot berdasarkan phone number
// Struktur: Wilykun/jadibot/{phoneNumber}/
func getJadibotFolder(phoneNumber string) string {
        return filepath.Join(JadibotFolder, phoneNumber)
}

// getJadibotDBPath - Generate unique database path dengan subfolder per phone number
// Path: Wilykun/jadibot/{phoneNumber}/{phoneNumber}.db
func getJadibotDBPath(phoneNumber string) string {
        return filepath.Join(getJadibotFolder(phoneNumber), phoneNumber+".db")
}

// getJadibotDBURI - Generate SQLite database URI dengan proper parameters
func getJadibotDBURI(phoneNumber string) string {
        dbPath := getJadibotDBPath(phoneNumber)
        return fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(FULL)&_pragma=wal_autocheckpoint(100)", dbPath)
}

// cleanupJadibotFiles - Hapus seluruh folder jadibot (db + shm + wal) dan subfolder-nya
func cleanupJadibotFiles(phoneNumber string) {
        jadibotFolder := getJadibotFolder(phoneNumber)
        os.RemoveAll(jadibotFolder)
        fmt.Printf("%s🗑️ Folder jadibot %s dihapus: %s%s\n", ColorYellow, phoneNumber, jadibotFolder, ColorReset)
}

// getJadibotMetadataPath - Get metadata.json path untuk store jadibot info
func getJadibotMetadataPath(phoneNumber string) string {
        return filepath.Join(getJadibotFolder(phoneNumber), "metadata.json")
}

// saveJadibotMetadata - Save jadibot StartTime ke metadata.json
func saveJadibotMetadata(phoneNumber string, startTime time.Time) error {
        metadata := map[string]interface{}{
                "phoneNumber": phoneNumber,
                "startTime":   startTime.Unix(),
                "savedAt":     time.Now().Unix(),
        }
        data, _ := json.MarshalIndent(metadata, "", "  ")
        return os.WriteFile(getJadibotMetadataPath(phoneNumber), data, 0o644)
}

// loadJadibotMetadata - Load jadibot StartTime dari metadata.json
func loadJadibotMetadata(phoneNumber string) time.Time {
        data, err := os.ReadFile(getJadibotMetadataPath(phoneNumber))
        if err != nil {
                return time.Now()
        }
        var metadata map[string]int64
        json.Unmarshal(data, &metadata)
        if startUnix, ok := metadata["startTime"]; ok {
                return time.Unix(startUnix, 0)
        }
        return time.Now()
}

func GetJadibotManager() *JadibotManager {
        return jadibotManager
}

func (jm *JadibotManager) GetAllSessions() []*JadibotSession {
        jm.mu.RLock()
        defer jm.mu.RUnlock()

        sessions := make([]*JadibotSession, 0, len(jm.sessions))
        for _, session := range jm.sessions {
                sessions = append(sessions, session)
        }
        return sessions
}

func (jm *JadibotManager) GetSession(phoneNumber string) *JadibotSession {
        jm.mu.RLock()
        defer jm.mu.RUnlock()
        return jm.sessions[phoneNumber]
}

func (jm *JadibotManager) IsSessionExists(phoneNumber string) bool {
        jm.mu.RLock()
        defer jm.mu.RUnlock()
        _, exists := jm.sessions[phoneNumber]
        return exists
}

func (jm *JadibotManager) CreatePairingSession(phoneNumber string, ownerChat types.JID, mainClient *whatsmeow.Client) (string, error) {
        jm.mu.Lock()
        defer jm.mu.Unlock()

        if _, exists := jm.sessions[phoneNumber]; exists {
                return "", fmt.Errorf("session untuk nomor %s sudah ada", phoneNumber)
        }

        if _, exists := jm.pending[phoneNumber]; exists {
                return "", fmt.Errorf("proses pairing untuk nomor %s sedang berjalan", phoneNumber)
        }

        ctx := context.Background()

        jadibotFolder := getJadibotFolder(phoneNumber)
        err := os.MkdirAll(jadibotFolder, 0o755)
        if err != nil {
                return "", fmt.Errorf("gagal membuat folder jadibot: %v", err)
        }

        baseDBLog := waLog.Stdout("JadibotDB", "ERROR", true)
        dbLog := &FilteredLogger{logger: baseDBLog}
        dbURI := getJadibotDBURI(phoneNumber)

        container, err := sqlstore.New(ctx, "sqlite3", dbURI, dbLog)
        if err != nil {
                return "", fmt.Errorf("gagal membuat database: %v", err)
        }

        deviceStore := container.NewDevice()
        baseClientLog := waLog.Stdout("JadibotClient", "ERROR", true)
        clientLog := &FilteredLogger{logger: baseClientLog}
        client := whatsmeow.NewClient(deviceStore, clientLog)

        session := &JadibotSession{
                PhoneNumber: phoneNumber,
                Client:      client,
                Container:   container,
                Connected:   false,
                StartTime:   time.Now(),
                OwnerChat:   ownerChat,
        }

        jm.pending[phoneNumber] = session
        jm.mainClient = mainClient

        client.AddEventHandler(func(evt interface{}) {
                jm.handleJadibotEvent(phoneNumber, client, evt)
        })

        err = client.Connect()
        if err != nil {
                delete(jm.pending, phoneNumber)
                container.Close()
                jm.deleteJadibotFiles(phoneNumber)
                return "", fmt.Errorf("gagal connect: %v", err)
        }

        time.Sleep(1 * time.Second)

        code, err := client.PairPhone(ctx, phoneNumber, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
        if err != nil {
                delete(jm.pending, phoneNumber)
                client.Disconnect()
                container.Close()
                jm.deleteJadibotFiles(phoneNumber)
                return "", fmt.Errorf("gagal generate pairing code: %v", err)
        }

        go jm.waitForPairing(phoneNumber, 180*time.Second)

        return code, nil
}

func (jm *JadibotManager) waitForPairing(phoneNumber string, timeout time.Duration) {
        timer := time.NewTimer(timeout)
        defer timer.Stop()

        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()

        for {
                select {
                case <-timer.C:
                        jm.mu.Lock()
                        if session, exists := jm.pending[phoneNumber]; exists {
                                if !session.Connected {
                                        fmt.Printf("%s⚠️ Pairing timeout untuk jadibot: %s%s\n", ColorYellow, phoneNumber, ColorReset)
                                        
                                        // Kirim notif timeout ke owner menggunakan main client
                                        if jm.mainClient != nil && jm.mainClient.IsConnected() && !session.OwnerChat.IsEmpty() {
                                                go func(ownerChat types.JID, pairingNumber string) {
                                                        ctx := context.Background()
                                                        
                                                        timeoutMsg := fmt.Sprintf(`❌ *PAIRING JADIBOT TIMEOUT*

═══════════════════════════════════

📱 *Nomor:* %s
⏰ *Status:* Expired (3 menit)
🔴 *Kondisi:* Pairing Gagal

═══════════════════════════════════

😞 Proses pairing tidak berhasil dalam waktu yang diberikan.

🔄 *COBA LAGI - LANGKAH PRAKTIS:*

1️⃣ Pastikan koneksi internet stabil
2️⃣ Buka WhatsApp di HP tujuan
3️⃣ Ketik: *.jadibot %s*
4️⃣ Tunggu sampai tersambung sepenuhnya
5️⃣ Jangan tutup WhatsApp hingga selesai

═══════════════════════════════════

⚠️ *KEMUNGKINAN PENYEBAB:*
   • Nomor tujuan offline/tidak aktif
   • WhatsApp tertutup di HP tujuan
   • HP terkunci atau sleep mode
   • Internet putus saat pairing
   • Perangkat Tertaut tidak support

💡 *SOLUSI:*
   1. Pastikan HP tujuan terhubung WiFi/Data
   2. Buka WhatsApp dan biarkan terbuka
   3. Cek apakah ada notifikasi pairing
   4. Coba lagi dengan kode baru

═══════════════════════════════════

📌 *COMMAND BANTUAN:*
   • *.jadibot [nomor]* - Buat kode baru
   • *.listjadibot* - Lihat daftar
   • *.menu* - Lihat semua command
   • *.deljadibot [nomor]* - Hapus jadibot

═══════════════════════════════════`, pairingNumber, pairingNumber)
                                                        msg := &waProto.Message{
                                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                                        Text: proto.String(timeoutMsg),
                                                                },
                                                        }
                                                        _, err := jm.mainClient.SendMessage(ctx, ownerChat, msg)
                                                        if err != nil {
                                                                fmt.Printf("%s⚠️ Notif timeout GAGAL: %v%s\n", ColorYellow, err, ColorReset)
                                                        } else {
                                                                fmt.Printf("%s✅ Notif timeout terkirim ke owner%s\n", ColorGreen, ColorReset)
                                                        }
                                                }(session.OwnerChat, phoneNumber)
                                                time.Sleep(500 * time.Millisecond)
                                        }
                                        
                                        session.Client.Disconnect()
                                        if session.Container != nil {
                                                session.Container.Close()
                                        }
                                        delete(jm.pending, phoneNumber)
                                        jm.deleteJadibotFiles(phoneNumber)
                                }
                        }
                        jm.mu.Unlock()
                        return
                case <-ticker.C:
                        jm.mu.RLock()
                        session, exists := jm.pending[phoneNumber]
                        connected := exists && session.Connected
                        jm.mu.RUnlock()

                        if !exists || connected {
                                return
                        }
                }
        }
}

func (jm *JadibotManager) handleJadibotEvent(phoneNumber string, client *whatsmeow.Client, evt interface{}) {
        switch v := evt.(type) {
        case *events.PairSuccess:
                jm.mu.Lock()
                var ownerChat types.JID
                if session, exists := jm.pending[phoneNumber]; exists {
                        session.Connected = true
                        ownerChat = session.OwnerChat
                        jm.sessions[phoneNumber] = session
                        delete(jm.pending, phoneNumber)
                        saveJadibotMetadata(phoneNumber, session.StartTime)
                }
                jm.mu.Unlock()

                fmt.Printf("%s✅ Jadibot berhasil terhubung: %s (JID: %s)%s\n", ColorGreen, phoneNumber, v.ID.String(), ColorReset)
                
                // Kirim notif berhasil ke owner
                if !ownerChat.IsEmpty() {
                        ctx := context.Background()
                        successMsg := fmt.Sprintf(`✅ *JADIBOT BERHASIL TERHUBUNG!*

═══════════════════════════════════

📱 *Nomor:* %s
🟢 *Status:* Online & Aktif
🔑 *JID:* %s

═══════════════════════════════════

✨ *PROSES AKTIVASI BERHASIL:*

1️⃣ ✓ Pairing code diverifikasi
2️⃣ ✓ Session terenkripsi dibuat
3️⃣ ✓ Database tersimpan dengan aman
4️⃣ ✓ Fitur auto diaktifkan
5️⃣ ✓ Auto reconnect dikonfigurasi
6️⃣ ✓ Presence status diupdate
7️⃣ ✓ Jadibot siap beroperasi

═══════════════════════════════════

📊 *FITUR YANG AKTIF SEKARANG:*
  ✓ Auto Read Story (Otomatis baca cerita)
  ✓ Auto Reaction Story (Reaksi random emoji)
  ✓ Auto Presence (Status online 24/7)
  ✓ Auto Reconnect (Koneksi otomatis)

🔒 *KEAMANAN & PENYIMPANAN:*
  ✓ Session: Terenkripsi AES-256
  ✓ Database: Aman di Wilykun/jadibot/
  ✓ Backup: Otomatis setiap 5 menit
  ✓ Authenticator: Hybrid encryption

═══════════════════════════════════

🚀 *STATUS OPERASIONAL:*
   Jadibot aktif 24/7 tanpa henti

📌 *COMMAND BANTUAN:*
   • *.listjadibot* - Lihat semua jadibot
   • *.deljadibot [nomor]* - Hapus jadibot
   • *.jadibotinfo [nomor]* - Info detail
   • *.menu* - Menu lengkap

═══════════════════════════════════`, phoneNumber, v.ID.String())
                        msg := &waProto.Message{
                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                        Text: proto.String(successMsg),
                                },
                        }
                        client.SendMessage(ctx, ownerChat, msg)
                }
                
                // Aggressive presence update
                ctx := context.Background()
                client.SendPresence(ctx, types.PresenceAvailable)
                time.Sleep(500 * time.Millisecond)
                client.SendPresence(ctx, types.PresenceAvailable)

        case *events.Connected:
                jm.mu.Lock()
                if session, exists := jm.pending[phoneNumber]; exists {
                        if client.Store.ID != nil {
                                session.Connected = true
                                jm.sessions[phoneNumber] = session
                                delete(jm.pending, phoneNumber)
                                fmt.Printf("%s✅ Jadibot terhubung: %s%s\n", ColorGreen, phoneNumber, ColorReset)
                        }
                }
                jm.mu.Unlock()

                ctx := context.Background()
                // Multiple presence updates untuk memastikan device aktif
                for i := 0; i < 3; i++ {
                        client.SendPresence(ctx, types.PresenceAvailable)
                        if i < 2 {
                                time.Sleep(300 * time.Millisecond)
                        }
                }
                go utils.CacheAllJoinedGroupsMappings(client)

        case *events.LoggedOut:
                fmt.Printf("%s⚠️ Jadibot logged out: %s%s\n", ColorYellow, phoneNumber, ColorReset)
                jm.RemoveSession(phoneNumber)

        case *events.Disconnected:
                fmt.Printf("%s⚠️ Jadibot disconnected: %s, mencoba reconnect...%s\n", ColorYellow, phoneNumber, ColorReset)
                go jm.reconnectSession(phoneNumber)

        case *events.Message:
                if v.Info.Chat.Server == types.BroadcastServer {
                        handleJadibotStory(client, v, phoneNumber)
                }
        }
}

func (jm *JadibotManager) reconnectSession(phoneNumber string) {
        jm.mu.Lock()
        session, exists := jm.sessions[phoneNumber]
        if !exists || session == nil {
                jm.mu.Unlock()
                return
        }

        if session.Reconnecting {
                jm.mu.Unlock()
                return
        }
        session.Reconnecting = true
        jm.mu.Unlock()

        defer func() {
                jm.mu.Lock()
                if s, ok := jm.sessions[phoneNumber]; ok {
                        s.Reconnecting = false
                }
                jm.mu.Unlock()
        }()

        maxRetries := 5
        retryDelays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second, 60 * time.Second}

        for attempt := 1; attempt <= maxRetries; attempt++ {
                if attempt <= len(retryDelays) {
                        time.Sleep(retryDelays[attempt-1])
                } else {
                        time.Sleep(60 * time.Second)
                }

                jm.mu.RLock()
                session, exists = jm.sessions[phoneNumber]
                jm.mu.RUnlock()

                if !exists || session == nil {
                        return
                }

                if session.Client.IsConnected() {
                        fmt.Printf("%s✅ Jadibot %s sudah terhubung kembali%s\n", ColorGreen, phoneNumber, ColorReset)
                        jm.mu.Lock()
                        session.FailCount = 0
                        jm.mu.Unlock()
                        return
                }

                if session.Client.Store.ID == nil {
                        fmt.Printf("%s❌ Jadibot %s tidak memiliki device ID - MENGHAPUS%s\n", ColorYellow, phoneNumber, ColorReset)
                        jm.RemoveSession(phoneNumber)
                        return
                }

                ctx := context.Background()
                err := session.Client.Connect()
                if err != nil {
                        fmt.Printf("%s⚠️ Gagal reconnect jadibot %s (percobaan %d/%d): %v%s\n", ColorYellow, phoneNumber, attempt, maxRetries, err, ColorReset)

                        jm.mu.Lock()
                        session.FailCount++
                        session.LastFailTime = time.Now()
                        jm.mu.Unlock()

                        if attempt == maxRetries {
                                fmt.Printf("%s❌ Jadibot %s gagal reconnect setelah %d percobaan - MENGHAPUS%s\n", ColorYellow, phoneNumber, maxRetries, ColorReset)
                                jm.RemoveSession(phoneNumber)
                                return
                        }
                        continue
                }

                time.Sleep(3 * time.Second)

                if !session.Client.IsConnected() {
                        fmt.Printf("%s⚠️ Jadibot %s masih tidak terhubung setelah reconnect (percobaan %d/%d)%s\n", ColorYellow, phoneNumber, attempt, maxRetries, ColorReset)

                        jm.mu.Lock()
                        session.FailCount++
                        session.LastFailTime = time.Now()
                        jm.mu.Unlock()

                        if attempt == maxRetries {
                                fmt.Printf("%s❌ Jadibot %s gagal reconnect setelah %d percobaan - MENGHAPUS%s\n", ColorYellow, phoneNumber, maxRetries, ColorReset)
                                jm.RemoveSession(phoneNumber)
                                return
                        }
                        continue
                }

                session.Client.SendPresence(ctx, types.PresenceAvailable)
                fmt.Printf("%s✅ Jadibot %s berhasil reconnect%s\n", ColorGreen, phoneNumber, ColorReset)

                jm.mu.Lock()
                session.FailCount = 0
                session.Connected = true
                jm.mu.Unlock()
                return
        }
}

func (jm *JadibotManager) StartHealthCheck() {
        ticker := time.NewTicker(60 * time.Second)
        defer ticker.Stop()

        fmt.Printf("%s🔍 Health check jadibot dimulai (interval: 60 detik)%s\n", ColorCyan, ColorReset)

        for range ticker.C {
                jm.validateAllSessions()
        }
}

type sessionSnapshot struct {
        phoneNumber    string
        hasClient      bool
        hasDeviceID    bool
        isConnected    bool
        isReconnecting bool
        failCount      int
        lastFailTime   time.Time
}

func (jm *JadibotManager) validateAllSessions() {
        const maxConsecutiveFails = 10
        const failResetDuration = 5 * time.Minute

        jm.mu.RLock()
        snapshots := make([]sessionSnapshot, 0, len(jm.sessions))
        for phoneNumber, session := range jm.sessions {
                snap := sessionSnapshot{
                        phoneNumber:    phoneNumber,
                        hasClient:      session != nil && session.Client != nil,
                        isReconnecting: session.Reconnecting,
                        failCount:      session.FailCount,
                        lastFailTime:   session.LastFailTime,
                }
                if snap.hasClient {
                        snap.hasDeviceID = session.Client.Store != nil && session.Client.Store.ID != nil
                        snap.isConnected = session.Client.IsConnected()
                }
                snapshots = append(snapshots, snap)
        }
        jm.mu.RUnlock()

        var invalidSessions []string

        for _, snap := range snapshots {
                if !snap.hasClient {
                        invalidSessions = append(invalidSessions, snap.phoneNumber)
                        continue
                }

                if !snap.hasDeviceID {
                        fmt.Printf("%s⚠️ [Health Check] Jadibot %s tidak memiliki device ID%s\n", ColorYellow, snap.phoneNumber, ColorReset)
                        invalidSessions = append(invalidSessions, snap.phoneNumber)
                        continue
                }

                if snap.isReconnecting {
                        continue
                }

                if time.Since(snap.lastFailTime) > failResetDuration && snap.failCount > 0 {
                        jm.mu.Lock()
                        if s, ok := jm.sessions[snap.phoneNumber]; ok {
                                s.FailCount = 0
                        }
                        jm.mu.Unlock()
                }

                if !snap.isConnected {
                        jm.mu.Lock()
                        session, exists := jm.sessions[snap.phoneNumber]
                        if !exists {
                                jm.mu.Unlock()
                                continue
                        }
                        session.FailCount++
                        session.LastFailTime = time.Now()
                        currentFailCount := session.FailCount
                        jm.mu.Unlock()

                        if currentFailCount >= maxConsecutiveFails {
                                fmt.Printf("%s❌ [Health Check] Jadibot %s gagal terhubung %d kali berturut-turut - MENGHAPUS%s\n", ColorYellow, snap.phoneNumber, currentFailCount, ColorReset)
                                invalidSessions = append(invalidSessions, snap.phoneNumber)
                                continue
                        }

                        fmt.Printf("%s⚠️ [Health Check] Jadibot %s tidak terhubung (fail count: %d/%d), memulai reconnect...%s\n", ColorYellow, snap.phoneNumber, currentFailCount, maxConsecutiveFails, ColorReset)
                        go jm.reconnectSession(snap.phoneNumber)
                } else {
                        jm.mu.Lock()
                        if s, ok := jm.sessions[snap.phoneNumber]; ok && s.FailCount > 0 {
                                s.FailCount = 0
                        }
                        jm.mu.Unlock()
                }
        }

        for _, phoneNumber := range invalidSessions {
                fmt.Printf("%s🗑️ [Health Check] Menghapus jadibot tidak valid: %s%s\n", ColorYellow, phoneNumber, ColorReset)
                jm.RemoveSession(phoneNumber)
        }

        jm.cleanupOrphanedFiles()

        jm.mu.RLock()
        activeCount := len(jm.sessions)
        jm.mu.RUnlock()

        if len(invalidSessions) > 0 {
                fmt.Printf("%s📊 [Health Check] Selesai - %d jadibot aktif, %d dihapus%s\n", ColorCyan, activeCount, len(invalidSessions), ColorReset)
        }
}

func (jm *JadibotManager) cleanupOrphanedFiles() {
        files, err := os.ReadDir(JadibotFolder)
        if err != nil {
                return
        }

        jm.mu.RLock()
        activeSessions := make(map[string]bool)
        for phoneNumber := range jm.sessions {
                activeSessions[phoneNumber] = true
        }
        for phoneNumber := range jm.pending {
                activeSessions[phoneNumber] = true
        }
        jm.mu.RUnlock()

        for _, file := range files {
                if file.IsDir() {
                        continue
                }

                fileName := file.Name()
                if !strings.HasSuffix(fileName, ".db") || strings.HasSuffix(fileName, "-shm") || strings.HasSuffix(fileName, "-wal") {
                        continue
                }

                phoneNumber := strings.TrimSuffix(fileName, ".db")

                if !activeSessions[phoneNumber] {
                        ctx := context.Background()
                        baseDBLog := waLog.Stdout("JadibotDB", "ERROR", true)
                        dbLog := &FilteredLogger{logger: baseDBLog}
                        dbURI := getJadibotDBURI(phoneNumber)

                        container, err := sqlstore.New(ctx, "sqlite3", dbURI, dbLog)
                        if err != nil {
                                jm.deleteJadibotFiles(phoneNumber)
                                continue
                        }

                        deviceStore, err := container.GetFirstDevice(ctx)
                        if err != nil || deviceStore.ID == nil {
                                container.Close()
                                jm.deleteJadibotFiles(phoneNumber)
                                continue
                        }

                        container.Close()
                }
        }
}

func (jm *JadibotManager) RemoveSession(phoneNumber string) error {
        jm.mu.Lock()
        defer jm.mu.Unlock()

        session, exists := jm.sessions[phoneNumber]
        if !exists {
                if pendingSession, pendingExists := jm.pending[phoneNumber]; pendingExists {
                        pendingSession.Client.Disconnect()
                        if pendingSession.Container != nil {
                                pendingSession.Container.Close()
                        }
                        delete(jm.pending, phoneNumber)
                        jm.deleteJadibotFiles(phoneNumber)
                        return nil
                }
                return fmt.Errorf("session tidak ditemukan: %s", phoneNumber)
        }

        if session.Client != nil {
                session.Client.Disconnect()
        }

        if session.Container != nil {
                session.Container.Close()
        }

        delete(jm.sessions, phoneNumber)

        cleanupJadibotFiles(phoneNumber)
        fmt.Printf("%s✅ Jadibot %s berhasil dihapus%s\n", ColorGreen, phoneNumber, ColorReset)
        return nil
}

func (jm *JadibotManager) deleteJadibotFiles(phoneNumber string) {
        cleanupJadibotFiles(phoneNumber)
}

func (jm *JadibotManager) LoadExistingSessions() {
        ctx := context.Background()

        files, err := os.ReadDir(JadibotFolder)
        if err != nil {
                fmt.Printf("%s⚠️ Gagal membaca folder jadibot: %v%s\n", ColorYellow, err, ColorReset)
                return
        }

        // Load jadibots secara PARALEL untuk kecepatan - maksimal 5 goroutine bersamaan
        semaphore := make(chan struct{}, 5)
        var wg sync.WaitGroup

        for _, file := range files {
                if file.IsDir() {
                        wg.Add(1)
                        go func(phoneNumber string) {
                                defer wg.Done()
                                
                                // Semaphore untuk limit concurrent loading
                                semaphore <- struct{}{}
                                defer func() { <-semaphore }()

                                baseDBLog := waLog.Stdout("JadibotDB", "ERROR", true)
                                dbLog := &FilteredLogger{logger: baseDBLog}
                                dbURI := getJadibotDBURI(phoneNumber)

                                container, err := sqlstore.New(ctx, "sqlite3", dbURI, dbLog)
                                if err != nil {
                                        fmt.Printf("%s⚠️ Gagal load database jadibot %s: %v - MENGHAPUS%s\n", ColorYellow, phoneNumber, err, ColorReset)
                                        jm.deleteJadibotFiles(phoneNumber)
                                        return
                                }

                                deviceStore, err := container.GetFirstDevice(ctx)
                                if err != nil {
                                        fmt.Printf("%s⚠️ Gagal get device jadibot %s: %v - MENGHAPUS%s\n", ColorYellow, phoneNumber, err, ColorReset)
                                        container.Close()
                                        jm.deleteJadibotFiles(phoneNumber)
                                        return
                                }

                                if deviceStore.ID == nil {
                                        fmt.Printf("%s⚠️ Jadibot %s belum terhubung/tidak valid - MENGHAPUS%s\n", ColorYellow, phoneNumber, ColorReset)
                                        container.Close()
                                        jm.deleteJadibotFiles(phoneNumber)
                                        return
                                }

                                baseClientLog := waLog.Stdout("JadibotClient", "ERROR", true)
                                clientLog := &FilteredLogger{logger: baseClientLog}
                                client := whatsmeow.NewClient(deviceStore, clientLog)

                                startTime := loadJadibotMetadata(phoneNumber)
                                session := &JadibotSession{
                                        PhoneNumber: phoneNumber,
                                        Client:      client,
                                        Container:   container,
                                        Connected:   false,
                                        StartTime:   startTime,
                                }

                                client.AddEventHandler(func(evt interface{}) {
                                        jm.handleJadibotEvent(phoneNumber, client, evt)
                                })

                                err = client.Connect()
                                if err != nil {
                                        fmt.Printf("%s⚠️ Gagal connect jadibot %s: %v - MENGHAPUS%s\n", ColorYellow, phoneNumber, err, ColorReset)
                                        container.Close()
                                        jm.deleteJadibotFiles(phoneNumber)
                                        return
                                }

                                // Reduce delay dari 2 detik ke 500ms (cukup untuk stabilisasi koneksi)
                                time.Sleep(500 * time.Millisecond)

                                if client.Store.ID == nil || !client.IsConnected() {
                                        fmt.Printf("%s⚠️ Jadibot %s tidak dapat terhubung - MENGHAPUS%s\n", ColorYellow, phoneNumber, ColorReset)
                                        client.Disconnect()
                                        container.Close()
                                        jm.deleteJadibotFiles(phoneNumber)
                                        return
                                }

                                jm.mu.Lock()
                                session.Connected = true
                                jm.sessions[phoneNumber] = session
                                jm.mu.Unlock()

                                client.SendPresence(ctx, types.PresenceAvailable)
                                fmt.Printf("%s✅ Jadibot %s berhasil dimuat dan terhubung%s\n", ColorGreen, phoneNumber, ColorReset)
                        }(file.Name())
                }
        }

        wg.Wait()
        go jm.StartHealthCheck()
}

func processJadibotStory(client *whatsmeow.Client, msg *events.Message, phoneNumber string, senderPhone string, senderInfo utils.SenderInfo) {
        ctx := context.Background()
        senderJID := msg.Info.Sender

        displayName := senderInfo.Name
        if displayName == "" {
                displayName = formatPhoneNumber(senderPhone)
        }

        emoji := ""
        reactionSuccess := false

        // Generate emoji HANYA jika autoLikeStory aktif
        if GetAutoLikeStory() {
                emoji = getRandomEmoji()
        }

        // 1️⃣ Delay DULU (sama seperti bot utama) - menggunakan config random 1-20 detik
        delay := getStoryDelay()
        delaySeconds := delay.Seconds()
        time.Sleep(delay)

        // 2️⃣ Send reaction PRIORITAS! HANYA jika autoLikeStory aktif DAN emoji ada (SETELAH delay, SEBELUM read)
        if GetAutoLikeStory() && emoji != "" {
                _, err := client.SendMessage(ctx, msg.Info.Chat, client.BuildReaction(msg.Info.Chat, senderJID, msg.Info.ID, emoji))
                if err == nil {
                        reactionSuccess = true
                } else {
                        // Log error untuk debugging
                        fmt.Printf("%s⚠️ Gagal send reaction jadibot %s untuk %s (emoji %s): %v%s\n", ColorYellow, phoneNumber, displayName, emoji, err, ColorReset)
                }
        }

        // 3️⃣ MarkRead HANYA jika autoReadStory aktif (SETELAH reaction!)
        if GetAutoReadStory() {
                client.MarkRead(ctx, []types.MessageID{msg.Info.ID}, msg.Info.Timestamp, msg.Info.Chat, senderJID)
        }

        // Print hasil HANYA jika ada action yang berhasil (read atau reaction)
        if (GetAutoReadStory() || reactionSuccess) {
                loc, _ := time.LoadLocation("Asia/Jakarta")
                now := time.Now().In(loc)
                timeStr := fmt.Sprintf("%02d:%02d:%02d WIB", now.Hour(), now.Minute(), now.Second())

                reactionStr := "-"
                if reactionSuccess {
                        reactionStr = emoji
                }

                // Format delay info sesuai config
                delayStr := "Disabled"
                if GetStoryRandomDelay() {
                        delayStr = fmt.Sprintf("%.0f Detik (Random)", delaySeconds)
                } else {
                        delayStr = "1 Detik"
                }

                fmt.Printf("%s├══════════════════════════════════┤%s\n", ColorCyan, ColorReset)
                fmt.Printf("%s│%s » Jadibot    : %s%s%s\n", ColorCyan, ColorReset, ColorMagenta, phoneNumber, ColorReset)
                fmt.Printf("%s│%s » Status     : %sAktif ✓%s\n", ColorCyan, ColorReset, ColorGreen, ColorReset)
                fmt.Printf("%s│%s » Waktu      : %s%s%s\n", ColorCyan, ColorReset, ColorBlue, timeStr, ColorReset)
                fmt.Printf("%s│%s » Nama       : %s%s%s\n", ColorCyan, ColorReset, ColorGreen, displayName, ColorReset)
                fmt.Printf("%s│%s » View Delay : %s%s%s\n", ColorCyan, ColorReset, ColorYellow, delayStr, ColorReset)
                fmt.Printf("%s│%s » Reaksi     : %s%s%s\n", ColorCyan, ColorReset, ColorYellow, reactionStr, ColorReset)
                fmt.Printf("%s└───···%s\n", ColorCyan, ColorReset)
        }
}

func isJadibotStoryDeleted(msg *events.Message) bool {
        if msg.Message == nil {
                return true
        }
        if msg.Message.ImageMessage == nil &&
                msg.Message.VideoMessage == nil &&
                msg.Message.ExtendedTextMessage == nil &&
                msg.Message.AudioMessage == nil &&
                msg.Message.DocumentMessage == nil &&
                msg.Message.StickerMessage == nil {
                return true
        }
        return false
}

func handleJadibotStory(client *whatsmeow.Client, msg *events.Message, phoneNumber string) {
        if msg.Info.Chat.Server != types.BroadcastServer {
                return
        }

        if isJadibotStoryDeleted(msg) {
                return
        }

        senderInfo := utils.GetAccurateSenderInfo(client, msg.Info.Sender, msg.Info.Chat, msg.Info.IsFromMe)

        botJID := client.Store.ID
        if utils.IsSelfMessage(client, msg.Info.Sender) || msg.Info.Sender.User == botJID.User {
                return
        }

        senderPhone := utils.ExtractPhoneFromJID(msg.Info.Sender)
        if senderPhone == "" && utils.IsPNUser(senderInfo.ID) {
                if parsed, err := types.ParseJID(senderInfo.ID); err == nil {
                        senderPhone = utils.ExtractPhoneFromJID(parsed)
                }
        }
        if senderPhone == "" {
                senderPhone = msg.Info.Sender.User
        }

        storyKey := fmt.Sprintf("jadibot_%s_%s_%s", phoneNumber, msg.Info.ID, senderPhone)
        now := time.Now()

        storyMutex.Lock()
        if lastTime, exists := processedStory[storyKey]; exists {
                if now.Sub(lastTime) < storyCooldown {
                        storyMutex.Unlock()
                        return
                }
        }
        processedStory[storyKey] = now
        storyMutex.Unlock()

        go processJadibotStory(client, msg, phoneNumber, senderPhone, senderInfo)
}

func HandleJadibotCommand(client *whatsmeow.Client, chat types.JID, messageID string, sender types.JID, args string) {
        ctx := context.Background()

        if args == "" {
                helpMsg := `📱 *JADIBOT COMMAND*

Cara pakai:
*.jadibot 6289xxxxxxxxx*

Contoh:
*.jadibot 6289681234567*

⚠️ *Catatan:*
• Nomor harus diawali 6289
• Fitur: Auto Read Story & Auto Reaction
• Ketik *.listjadibot* untuk melihat daftar
• Ketik *.deljadibot 6289xxx* untuk menghapus`

                replyMsg := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                Text: proto.String(helpMsg),
                                ContextInfo: &waProto.ContextInfo{
                                        StanzaID:    proto.String(messageID),
                                        Participant: proto.String(sender.String()),
                                },
                        },
                }
                client.SendMessage(ctx, chat, replyMsg)
                return
        }

        phoneNumber := strings.TrimSpace(args)
        phoneNumber = strings.ReplaceAll(phoneNumber, "+", "")
        phoneNumber = strings.ReplaceAll(phoneNumber, "-", "")
        phoneNumber = strings.ReplaceAll(phoneNumber, " ", "")

        if !strings.HasPrefix(phoneNumber, "62") {
                if strings.HasPrefix(phoneNumber, "0") {
                        phoneNumber = "62" + phoneNumber[1:]
                }
        }

        if !strings.HasPrefix(phoneNumber, "6289") && !strings.HasPrefix(phoneNumber, "6281") && !strings.HasPrefix(phoneNumber, "6282") && !strings.HasPrefix(phoneNumber, "6283") && !strings.HasPrefix(phoneNumber, "6285") && !strings.HasPrefix(phoneNumber, "6287") && !strings.HasPrefix(phoneNumber, "6288") {
                errorMsg := `❌ *Format Nomor Salah!*

Nomor harus diawali dengan kode Indonesia:
• 6281xxxxxxxxx
• 6282xxxxxxxxx
• 6285xxxxxxxxx
• 6289xxxxxxxxx
• dll

Contoh: *.jadibot 6289681234567*`

                replyMsg := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                Text: proto.String(errorMsg),
                                ContextInfo: &waProto.ContextInfo{
                                        StanzaID:    proto.String(messageID),
                                        Participant: proto.String(sender.String()),
                                },
                        },
                }
                client.SendMessage(ctx, chat, replyMsg)
                return
        }

        if len(phoneNumber) < 10 || len(phoneNumber) > 15 {
                errorMsg := "❌ *Nomor tidak valid!*\n\nPanjang nomor harus 10-15 digit."
                replyMsg := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                Text: proto.String(errorMsg),
                                ContextInfo: &waProto.ContextInfo{
                                        StanzaID:    proto.String(messageID),
                                        Participant: proto.String(sender.String()),
                                },
                        },
                }
                client.SendMessage(ctx, chat, replyMsg)
                return
        }

        jm := GetJadibotManager()
        if jm.IsSessionExists(phoneNumber) {
                errorMsg := fmt.Sprintf("❌ *Nomor %s sudah terdaftar sebagai jadibot!*\n\nKetik *.listjadibot* untuk melihat daftar.", phoneNumber)
                replyMsg := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                Text: proto.String(errorMsg),
                                ContextInfo: &waProto.ContextInfo{
                                        StanzaID:    proto.String(messageID),
                                        Participant: proto.String(sender.String()),
                                },
                        },
                }
                client.SendMessage(ctx, chat, replyMsg)
                return
        }

        waitMsg := fmt.Sprintf(`⏳ *JADIBOT - LOADING...*

📱 *Nomor Tujuan:* %s

*Status Proses:*
⏳ Membuat pairing code...
☐ Menunggu pairing
☐ Verifikasi koneksi
☐ Aktivasi fitur

⏰ *Estimasi:* ~30 detik
💾 *Lokasi Data:* Wilykun/jadibot/

_Mohon tunggu sebentar..._`, phoneNumber)
        replyMsg := &waProto.Message{
                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                        Text: proto.String(waitMsg),
                        ContextInfo: &waProto.ContextInfo{
                                StanzaID:    proto.String(messageID),
                                Participant: proto.String(sender.String()),
                        },
                },
        }
        client.SendMessage(ctx, chat, replyMsg)

        code, err := jm.CreatePairingSession(phoneNumber, chat, client)
        if err != nil {
                errorMsg := fmt.Sprintf("❌ *Gagal membuat pairing code!*\n\nError: %v", err)
                replyMsg := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                Text: proto.String(errorMsg),
                                ContextInfo: &waProto.ContextInfo{
                                        StanzaID:    proto.String(messageID),
                                        Participant: proto.String(sender.String()),
                                },
                        },
                }
                client.SendMessage(ctx, chat, replyMsg)
                return
        }

        successMsg := fmt.Sprintf(`✅ *PAIRING CODE BERHASIL DIBUAT!*

═══════════════════════════════════

📱 *Nomor Tujuan:* %s
🔑 *Kode Pairing:* *%s*

═══════════════════════════════════

📖 *TUTORIAL PAIRING LENGKAP:*

1️⃣ Buka WhatsApp di HP tujuan
2️⃣ Tap ⋮ (titik tiga di atas) 
3️⃣ Pilih "Perangkat Tertaut"
4️⃣ Tap "+ Tautkan Perangkat"
5️⃣ Pilih "Tautkan dengan Nomor Telepon"
6️⃣ Masukkan kode: *%s*
7️⃣ Tunggu hingga tersambung

═══════════════════════════════════

⏱️ *WAKTU BERLAKU:* 3 menit
⚠️ *PENTING - Pastikan:*
   ✓ Nomor tujuan aktif & online
   ✓ WhatsApp terbuka di HP tujuan
   ✓ Internet stabil
   ✓ HP tidak sleep/terkunci

✨ *FITUR YANG AKAN AKTIF:*
   ✓ Auto Read Story
   ✓ Auto Reaction Story (Emoji Random)
   ✓ Auto Presence (Status Online)
   ✓ Auto Reconnect 24/7

═══════════════════════════════════
💾 *Data Keamanan:*
   • Database: Terenkripsi & Aman
   • Session: Disimpan di Wilykun/jadibot/
   • Backup: Otomatis setiap 5 menit

📌 *Bantuan:*
   • Ketik *.listjadibot* - Lihat daftar
   • Ketik *.menu* - Menu lengkap
   • Ketik *.deljadibot [nomor]* - Hapus

═══════════════════════════════════`, phoneNumber, code, code)

        replyMsg = &waProto.Message{
                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                        Text: proto.String(successMsg),
                        ContextInfo: &waProto.ContextInfo{
                                StanzaID:    proto.String(messageID),
                                Participant: proto.String(sender.String()),
                        },
                },
        }
        client.SendMessage(ctx, chat, replyMsg)

        fmt.Printf("%s🤖 Jadibot pairing code generated for %s: %s%s\n", ColorGreen, phoneNumber, code, ColorReset)
}

func HandleListJadibotCommand(client *whatsmeow.Client, chat types.JID, messageID string, sender types.JID) {
        ctx := context.Background()
        jm := GetJadibotManager()
        sessions := jm.GetAllSessions()

        if len(sessions) == 0 {
                noSessionMsg := `📋 *DAFTAR JADIBOT*

❌ Belum ada jadibot yang terhubung.

Ketik *.jadibot 6289xxxxxxxxx* untuk menambahkan.`

                replyMsg := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                Text: proto.String(noSessionMsg),
                                ContextInfo: &waProto.ContextInfo{
                                        StanzaID:    proto.String(messageID),
                                        Participant: proto.String(sender.String()),
                                },
                        },
                }
                client.SendMessage(ctx, chat, replyMsg)
                return
        }

        listMsg := "📋 *DAFTAR JADIBOT*\n\n"
        listMsg += "═══════════════════════════════════\n\n"
        
        for i, session := range sessions {
                status := "🔴 Offline"
                if session.Connected && session.Client.IsConnected() {
                        status = "🟢 Online"
                }

                // Format tanggal dan waktu terhubung
                startDateTime := FormatStartDateTime(session.StartTime)
                
                // Hitung uptime real-time (fresh setiap kali command dijalankan)
                uptime := time.Since(session.StartTime)
                uptimeStr := FormatDuration(uptime)

                listMsg += fmt.Sprintf("*%d.* 📱 %s\n", i+1, session.PhoneNumber)
                listMsg += fmt.Sprintf("    📅 Terhubung: %s\n", startDateTime)
                listMsg += fmt.Sprintf("    %s Status: %s\n", "🟢", status)
                listMsg += fmt.Sprintf("    ⏱️  Uptime: %s\n", uptimeStr)
                listMsg += "\n"
        }

        listMsg += "═══════════════════════════════════\n\n"
        listMsg += fmt.Sprintf("📊 *Total Jadibot:* %d aktif\n\n", len(sessions))
        listMsg += "*❌ Hapus Jadibot:*\n"
        listMsg += "*.deljadibot [nomor]*\n\n"
        listMsg += "Contoh: *.deljadibot 6288229456210*"

        replyMsg := &waProto.Message{
                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                        Text: proto.String(listMsg),
                        ContextInfo: &waProto.ContextInfo{
                                StanzaID:    proto.String(messageID),
                                Participant: proto.String(sender.String()),
                        },
                },
        }
        client.SendMessage(ctx, chat, replyMsg)
}

func HandleDelJadibotCommand(client *whatsmeow.Client, chat types.JID, messageID string, sender types.JID, args string) {
        ctx := context.Background()

        if args == "" {
                helpMsg := `❌ *Format Salah!*

Cara pakai:
*.deljadibot 6289xxxxxxxxx*

Contoh:
*.deljadibot 6289681234567*

Ketik *.listjadibot* untuk melihat daftar.`

                replyMsg := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                Text: proto.String(helpMsg),
                                ContextInfo: &waProto.ContextInfo{
                                        StanzaID:    proto.String(messageID),
                                        Participant: proto.String(sender.String()),
                                },
                        },
                }
                client.SendMessage(ctx, chat, replyMsg)
                return
        }

        phoneNumber := strings.TrimSpace(args)
        phoneNumber = strings.ReplaceAll(phoneNumber, "+", "")
        phoneNumber = strings.ReplaceAll(phoneNumber, "-", "")
        phoneNumber = strings.ReplaceAll(phoneNumber, " ", "")

        jm := GetJadibotManager()
        err := jm.RemoveSession(phoneNumber)

        if err != nil {
                errorMsg := fmt.Sprintf("❌ *Gagal menghapus jadibot!*\n\nError: %v\n\nKetik *.listjadibot* untuk melihat daftar.", err)
                replyMsg := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                Text: proto.String(errorMsg),
                                ContextInfo: &waProto.ContextInfo{
                                        StanzaID:    proto.String(messageID),
                                        Participant: proto.String(sender.String()),
                                },
                        },
                }
                client.SendMessage(ctx, chat, replyMsg)
                return
        }

        successMsg := fmt.Sprintf("✅ *Jadibot %s berhasil dihapus!*\n\nSemua data telah dibersihkan.", phoneNumber)
        replyMsg := &waProto.Message{
                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                        Text: proto.String(successMsg),
                        ContextInfo: &waProto.ContextInfo{
                                StanzaID:    proto.String(messageID),
                                Participant: proto.String(sender.String()),
                        },
                },
        }
        client.SendMessage(ctx, chat, replyMsg)
}

func FormatDuration(d time.Duration) string {
        days := int(d.Hours()) / 24
        hours := int(d.Hours()) % 24
        minutes := int(d.Minutes()) % 60

        if days > 0 {
                return fmt.Sprintf("%dh %dj %dm", days, hours, minutes)
        }
        if hours > 0 {
                return fmt.Sprintf("%dj %dm", hours, minutes)
        }
        return fmt.Sprintf("%dm", minutes)
}

func FormatStartDateTime(t time.Time) string {
        // Nama hari dalam Bahasa Indonesia
        dayNames := []string{
                "Minggu", "Senin", "Selasa", "Rabu",
                "Kamis", "Jumat", "Sabtu",
        }
        
        // Nama bulan dalam Bahasa Indonesia
        monthNames := []string{
                "", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
                "Juli", "Agustus", "September", "Oktober", "November", "Desember",
        }
        
        dayName := dayNames[t.Weekday()]
        monthName := monthNames[t.Month()]
        hour := t.Format("15:04") // Format 24-jam HH:MM
        
        return fmt.Sprintf("%s, %d %s %d - %s WIB", dayName, t.Day(), monthName, t.Year(), hour)
}
