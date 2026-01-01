package main

import (
        "context"
        "database/sql"
        "fmt"
        "os"
        "os/signal"
        "strings"
        "syscall"
        "time"

        "github.com/mdp/qrterminal/v3"
        _ "github.com/ncruces/go-sqlite3/driver"
        _ "github.com/ncruces/go-sqlite3/embed"
        "github.com/nyaruka/phonenumbers"
        "go.mau.fi/whatsmeow"
        waProto "go.mau.fi/whatsmeow/binary/proto"
        "go.mau.fi/whatsmeow/store/sqlstore"
        "go.mau.fi/whatsmeow/types"
        "go.mau.fi/whatsmeow/types/events"
        waLog "go.mau.fi/whatsmeow/util/log"
        "google.golang.org/protobuf/proto"

        "whatsapp-bot/commands"
        "whatsapp-bot/core"
        "whatsapp-bot/features"
        "whatsapp-bot/utils"
)

const (
        ColorReset   = "\033[0m"
        ColorCyan    = "\033[36m"
        ColorGreen   = "\033[32m"
        ColorYellow  = "\033[33m"
        ColorPurple  = "\033[35m"
        ColorMagenta = "\033[35m"
        ColorBlue    = "\033[34m"
        ColorBold    = "\033[1m"
)

type Ev struct {
        ChatPresence *events.ChatPresence
        Message      *events.Message
        Receipt      *events.Receipt
        More         interface{}
}

var (
        botStartTime time.Time
)

type FilteredLogger struct {
        logger waLog.Logger
}

func (f *FilteredLogger) Errorf(msg string, args ...interface{}) {
        formattedMsg := fmt.Sprintf(msg, args...)
        if strings.Contains(formattedMsg, "Failed to handle retry receipt") ||
                strings.Contains(formattedMsg, "Unable to verify ciphertext mac") ||
                strings.Contains(formattedMsg, "mismatching MAC") {
                return
        }
        f.logger.Errorf(msg, args...)
}

func (f *FilteredLogger) Warnf(msg string, args ...interface{}) {
        formattedMsg := fmt.Sprintf(msg, args...)
        if strings.Contains(formattedMsg, "mismatching MAC") ||
                strings.Contains(formattedMsg, "failed to verify ciphertext MAC") {
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

func Connect(nomor string, cb func(conn *whatsmeow.Client, evt Ev), useQR bool) {
        ctx := context.Background()

        // Buat folder Wilykun dan Wilykun/bossbot untuk bot utama
        err := os.MkdirAll("Wilykun/bossbot", 0o755)
        if err != nil {
                fmt.Println("GoError creating Wilykun/bossbot folder:", err)
                return
        }

        dbLog := waLog.Stdout("Database", "ERROR", true)
        dbPath := "file:Wilykun/bossbot/" + nomor + ".db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(FULL)&_pragma=wal_autocheckpoint(100)"
        container, err := sqlstore.New(ctx, "sqlite3", dbPath, dbLog)
        if err != nil {
                fmt.Println("GoError:", err)
                return
        }
        deviceStore, err := container.GetFirstDevice(ctx)
        if err != nil {
                fmt.Println("GoError:", err)
                return
        }

        baseClientLog := waLog.Stdout("Client", "ERROR", true)
        clientLog := &FilteredLogger{logger: baseClientLog}

        var client *whatsmeow.Client
        var reconnectAttempts int
        maxReconnectDelay := 60 * time.Second
        reconnectChan := make(chan bool, 10)

        var connectWithRetry func() error
        connectWithRetry = func() error {
                if client == nil {
                        client = whatsmeow.NewClient(deviceStore, clientLog)

                        client.AddEventHandler(func(evt interface{}) {
                                switch v := evt.(type) {
                                case *events.Message:
                                        cb(client, Ev{Message: v})
                                case *events.Receipt:
                                        cb(client, Ev{Receipt: v})
                                case *events.ChatPresence:
                                        cb(client, Ev{ChatPresence: v})
                                case *events.LoggedOut:
                                        fmt.Println("Bot logged out! Reason:", v.Reason)
                                        if v.OnConnect {
                                                fmt.Println("Logged out during connection, will retry in 5 seconds...")
                                                select {
                                                case reconnectChan <- true:
                                                default:
                                                }
                                        }
                                case *events.StreamReplaced:
                                        fmt.Println("Stream replaced, reconnecting in 2 seconds...")
                                        select {
                                        case reconnectChan <- true:
                                        default:
                                        }
                                case *events.Disconnected:
                                        fmt.Println("Disconnected from WhatsApp")
                                case *events.Connected:
                                        fmt.Println("Successfully connected to WhatsApp")
                                        reconnectAttempts = 0
                                        go utils.CacheAllJoinedGroupsMappings(client)
                                default:
                                        cb(client, Ev{More: evt})
                                }
                        })
                }

                if client.Store.ID == nil {
                        fmt.Println("No session found, pairing device...")

                        if useQR {
                                fmt.Print(ColorBold + ColorGreen + "\n📱 QR CODE METHOD\n" + ColorReset)
                                fmt.Print(ColorCyan + "Tunjukkan QR code ke kamera HP Anda...\n\n" + ColorReset)

                                qrChan, err := client.GetQRChannel(ctx)
                                if err != nil {
                                        return fmt.Errorf("QR channel error: %v", err)
                                }

                                err = client.Connect()
                                if err != nil {
                                        return fmt.Errorf("connection error: %v", err)
                                }

                                qrShown := false
                                for evt := range qrChan {
                                        if evt.Event == "code" {
                                                if !qrShown {
                                                        fmt.Print(formatQRTutorial(nomor))
                                                        qrShown = true
                                                }
                                                fmt.Print("\n" + ColorBold + ColorGreen + "🔄 QR Code Baru (Scan dengan HP Anda):\n\n" + ColorReset)
                                                config := qrterminal.Config{
                                                        HalfBlocks: true,
                                                        Level:      qrterminal.L,
                                                        Writer:     os.Stdout,
                                                        QuietZone:  1,
                                                }
                                                qrterminal.GenerateWithConfig(evt.Code, config)
                                                fmt.Print("\n")
                                        } else if evt.Event == "success" {
                                                fmt.Print(ColorBold + ColorGreen + "\n✅ Pairing dengan QR Code berhasil!\n" + ColorReset)
                                                break
                                        } else if evt.Event == "timeout" {
                                                return fmt.Errorf("QR code timeout")
                                        } else if evt.Event != "" {
                                                fmt.Print(ColorYellow + "⚠️ Event: " + evt.Event + ColorReset + "\n")
                                                if evt.Error != nil {
                                                        return fmt.Errorf("QR pairing error: %v", evt.Error)
                                                }
                                        }
                                }
                        } else {
                                err := client.Connect()
                                if err != nil {
                                        return fmt.Errorf("connection error: %v", err)
                                }
                                linkingCode, gagal := client.PairPhone(ctx, nomor, false, whatsmeow.PairClientChrome, "Chrome (Linux)")
                                if gagal != nil {
                                        return fmt.Errorf("pairing error: %v", gagal)
                                }
                                fmt.Print(formatConnectionMessage(nomor, linkingCode))
                        }
                } else {
                        if !client.IsConnected() {
                                err := client.Connect()
                                if err != nil {
                                        return fmt.Errorf("connection error: %v", err)
                                }
                                fmt.Print(formatConnectionMessage(nomor, "Connected"))
                                client.SendPresence(ctx, types.PresenceAvailable)
                        } else {
                                fmt.Print(formatConnectionMessage(nomor, "Already connected"))
                        }
                }
                return nil
        }

        for {
                err := connectWithRetry()
                if err == nil {
                        break
                }

                reconnectAttempts++
                delay := time.Duration(reconnectAttempts) * 5 * time.Second
                if delay > maxReconnectDelay {
                        delay = maxReconnectDelay
                }

                fmt.Printf("Connection failed (attempt %d): %v. Retrying in %v...\n", reconnectAttempts, err, delay)
                time.Sleep(delay)
        }

        go func() {
                ticker := time.NewTicker(30 * time.Second)
                checkpointTicker := time.NewTicker(5 * time.Minute)
                defer ticker.Stop()
                defer checkpointTicker.Stop()

                for {
                        select {
                        case <-ticker.C:
                                if client != nil && !client.IsConnected() {
                                        fmt.Println("Connection lost detected by health check, attempting to reconnect...")
                                        reconnectAttempts = 0
                                        for i := 0; i < 5; i++ {
                                                if err := connectWithRetry(); err == nil {
                                                        break
                                                }
                                                reconnectAttempts++
                                                delay := time.Duration(reconnectAttempts) * 5 * time.Second
                                                if delay > maxReconnectDelay {
                                                        delay = maxReconnectDelay
                                                }
                                                fmt.Printf("Reconnection failed (attempt %d), retrying in %v...\n", reconnectAttempts, delay)
                                                time.Sleep(delay)
                                        }
                                }
                        case <-checkpointTicker.C:
                                checkpointDatabase(nomor)
                        case <-reconnectChan:
                                time.Sleep(3 * time.Second)
                                fmt.Println("Reconnecting after event trigger...")
                                reconnectAttempts = 0
                                for i := 0; i < 5; i++ {
                                        if err := connectWithRetry(); err == nil {
                                                break
                                        }
                                        reconnectAttempts++
                                        delay := time.Duration(reconnectAttempts) * 5 * time.Second
                                        if delay > maxReconnectDelay {
                                                delay = maxReconnectDelay
                                        }
                                        fmt.Printf("Reconnection failed (attempt %d), retrying in %v...\n", reconnectAttempts, delay)
                                        time.Sleep(delay)
                                }
                        }
                }
        }()
}

func parseChoice(input string, maxOptions int) int {
        var choice int
        _, err := fmt.Sscanf(input, "%d", &choice)
        if err != nil || choice < 1 || choice > maxOptions {
                return -1
        }
        return choice - 1
}

func censorNumber(number string) string {
        if len(number) <= 9 {
                return number
        }
        firstDigits := 5
        lastDigits := 4
        stars := len(number) - firstDigits - lastDigits
        if stars < 1 {
                return number
        }
        censored := number[:firstDigits]
        for i := 0; i < stars; i++ {
                censored += "*"
        }
        censored += number[len(number)-lastDigits:]
        return censored
}

type CountryInfo struct {
        CountryCode string
        CountryName string
        Flag        string
}

var countryNameMap = map[string]string{
        "ID": "🇮🇩 Indonesia",
        "US": "🇺🇸 United States",
        "GB": "🇬🇧 United Kingdom",
        "FR": "🇫🇷 France",
        "DE": "🇩🇪 Germany",
        "IT": "🇮🇹 Italy",
        "ES": "🇪🇸 Spain",
        "NL": "🇳🇱 Netherlands",
        "BE": "🇧🇪 Belgium",
        "CH": "🇨🇭 Switzerland",
        "AT": "🇦🇹 Austria",
        "PL": "🇵🇱 Poland",
        "SE": "🇸🇪 Sweden",
        "NO": "🇳🇴 Norway",
        "DK": "🇩🇰 Denmark",
        "FI": "🇫🇮 Finland",
        "RU": "🇷🇺 Russia",
        "UA": "🇺🇦 Ukraine",
        "TR": "🇹🇷 Turkey",
        "GR": "🇬🇷 Greece",
        "PT": "🇵🇹 Portugal",
        "CZ": "🇨🇿 Czech Republic",
        "HU": "🇭🇺 Hungary",
        "RO": "🇷🇴 Romania",
        "BG": "🇧🇬 Bulgaria",
        "HR": "🇭🇷 Croatia",
        "SI": "🇸🇮 Slovenia",
        "SK": "🇸🇰 Slovakia",
        "LT": "🇱🇹 Lithuania",
        "LV": "🇱🇻 Latvia",
        "EE": "🇪🇪 Estonia",
        "JP": "🇯🇵 Japan",
        "KR": "🇰🇷 South Korea",
        "CN": "🇨🇳 China",
        "IN": "🇮🇳 India",
        "TH": "🇹🇭 Thailand",
        "MY": "🇲🇾 Malaysia",
        "SG": "🇸🇬 Singapore",
        "PH": "🇵🇭 Philippines",
        "VN": "🇻🇳 Vietnam",
        "BD": "🇧🇩 Bangladesh",
        "PK": "🇵🇰 Pakistan",
        "AU": "🇦🇺 Australia",
        "NZ": "🇳🇿 New Zealand",
        "CA": "🇨🇦 Canada",
        "MX": "🇲🇽 Mexico",
        "BR": "🇧🇷 Brazil",
        "AR": "🇦🇷 Argentina",
        "CL": "🇨🇱 Chile",
        "CO": "🇨🇴 Colombia",
        "PE": "🇵🇪 Peru",
        "ZA": "🇿🇦 South Africa",
        "EG": "🇪🇬 Egypt",
        "NG": "🇳🇬 Nigeria",
        "KE": "🇰🇪 Kenya",
}

func getCountryInfo(nomor string) CountryInfo {
        phoneNumber := "+" + nomor

        parsedNumber, err := phonenumbers.Parse(phoneNumber, "")
        if err != nil {
                return CountryInfo{
                        CountryCode: "??",
                        CountryName: "Unknown",
                        Flag:        "❓",
                }
        }

        countryCode := phonenumbers.GetRegionCodeForNumber(parsedNumber)
        countryName, exists := countryNameMap[countryCode]
        if !exists {
                countryName = "🌍 " + countryCode
        }

        return CountryInfo{
                CountryCode: countryCode,
                CountryName: countryName,
        }
}

func formatQRTutorial(nomor string) string {
        now := time.Now()
        months := []string{
                "Januari", "Februari", "Maret", "April", "Mei", "Juni",
                "Juli", "Agustus", "September", "Oktober", "November", "Desember",
        }
        dateStr := fmt.Sprintf("%d %s %d", now.Day(), months[now.Month()-1], now.Year())

        countryInfo := getCountryInfo(nomor)

        var message strings.Builder
        message.WriteString("\n")
        message.WriteString(strings.Repeat("=", 50) + "\n")
        message.WriteString(ColorBold + ColorCyan + "🤖 WhatsApp Auto-React Bot" + ColorReset + "\n")
        message.WriteString(strings.Repeat("=", 50) + "\n")
        message.WriteString(ColorPurple + "Developer: " + ColorReset + ColorBold + "Bang Wily" + ColorReset + "\n")
        message.WriteString(ColorCyan + "Tanggal: " + ColorReset + dateStr + "\n")
        message.WriteString(ColorYellow + "Nomor: " + ColorReset + ColorBold + nomor + ColorReset + "\n")
        message.WriteString(ColorGreen + "Negara: " + ColorReset + ColorBold + countryInfo.CountryName + ColorReset + "\n")
        message.WriteString("\n")

        message.WriteString(ColorBold + ColorGreen + "📱 METODE QR CODE" + ColorReset + "\n")
        message.WriteString(strings.Repeat("-", 50) + "\n")
        message.WriteString(ColorBold + ColorYellow + "📖 CARA SCAN QR CODE:" + ColorReset + "\n\n")
        message.WriteString(ColorCyan + "1." + ColorReset + " Ambil " + ColorBold + "smartphone Anda" + ColorReset + " yang sudah login WhatsApp\n")
        message.WriteString(ColorCyan + "2." + ColorReset + " Buka aplikasi " + ColorBold + "WhatsApp" + ColorReset + "\n")
        message.WriteString(ColorCyan + "3." + ColorReset + " Tap " + ColorBold + ColorGreen + "⋮ (tiga titik)" + ColorReset + " di pojok kanan atas\n")
        message.WriteString(ColorCyan + "4." + ColorReset + " Pilih " + ColorBold + ColorGreen + "\"Perangkat Tertaut\"" + ColorReset + "\n")
        message.WriteString(ColorCyan + "5." + ColorReset + " Tap " + ColorBold + ColorGreen + "\"Tautkan Perangkat\"" + ColorReset + "\n")
        message.WriteString(ColorCyan + "6." + ColorReset + " " + ColorBold + "SCAN QR CODE DI BAWAH" + ColorReset + " dengan kamera HP Anda\n")
        message.WriteString(ColorCyan + "7." + ColorReset + " Tunggu hingga " + ColorBold + ColorGreen + "berhasil terhubung" + ColorReset + "\n\n")

        message.WriteString(ColorBold + ColorYellow + "⏱️ CATATAN PENTING:" + ColorReset + "\n")
        message.WriteString(ColorCyan + "• " + ColorReset + "QR Code akan " + ColorBold + "berubah setiap 30 detik" + ColorReset + "\n")
        message.WriteString(ColorCyan + "• " + ColorReset + "Pastikan HP " + ColorBold + "terhubung Internet" + ColorReset + "\n")
        message.WriteString(ColorCyan + "• " + ColorReset + "Jangan tutup layar saat proses berlangsung\n")
        message.WriteString(ColorCyan + "• " + ColorReset + "Pastikan WhatsApp di HP adalah " + ColorBold + "versi terbaru" + ColorReset + "\n\n")

        message.WriteString(strings.Repeat("=", 50) + "\n")

        return message.String()
}

func formatConnectionMessage(nomor string, code string) string {
        now := time.Now()
        months := []string{
                "Januari", "Februari", "Maret", "April", "Mei", "Juni",
                "Juli", "Agustus", "September", "Oktober", "November", "Desember",
        }
        dateStr := fmt.Sprintf("%d %s %d", now.Day(), months[now.Month()-1], now.Year())

        countryInfo := getCountryInfo(nomor)

        var message strings.Builder
        message.WriteString("\n")

        message.WriteString(ColorBold + ColorCyan + "🤖 WhatsApp Auto-React Bot" + ColorReset + "\n")
        message.WriteString(strings.Repeat("=", 50) + "\n")
        message.WriteString(ColorPurple + "Developer: " + ColorReset + ColorBold + "Bang Wily" + ColorReset + "\n")
        message.WriteString(ColorCyan + "Tanggal: " + ColorReset + dateStr + "\n")
        message.WriteString(ColorYellow + "Nomor: " + ColorReset + ColorBold + nomor + ColorReset + "\n")
        message.WriteString(ColorGreen + "Negara: " + ColorReset + ColorBold + countryInfo.CountryName + ColorReset + "\n")
        message.WriteString(strings.Repeat("=", 50) + "\n")

        if code != "Connected" && code != "Already connected" {
                message.WriteString("\n")
                message.WriteString(ColorBold + ColorGreen + "📱 KODE PAIRING: " + code + ColorReset + "\n")
                message.WriteString("\n")
                message.WriteString(ColorBold + ColorYellow + "📖 TUTORIAL LENGKAP:" + ColorReset + "\n")
                message.WriteString(ColorCyan + "1." + ColorReset + " Buka aplikasi " + ColorBold + "WhatsApp" + ColorReset + " di HP Anda\n")
                message.WriteString(ColorCyan + "2." + ColorReset + " Klik " + ColorBold + ColorGreen + "titik tiga" + ColorReset + " (⋮) di pojok kanan atas\n")
                message.WriteString(ColorCyan + "3." + ColorReset + " Pilih menu " + ColorBold + ColorGreen + "\"Perangkat Tertaut\"" + ColorReset + "\n")
                message.WriteString(ColorCyan + "4." + ColorReset + " Klik tombol " + ColorBold + ColorGreen + "\"Tautkan Perangkat\"" + ColorReset + "\n")
                message.WriteString(ColorCyan + "5." + ColorReset + " Pilih " + ColorBold + ColorGreen + "\"Tautkan dengan Nomor Telepon\"" + ColorReset + "\n")
                message.WriteString(ColorCyan + "6." + ColorReset + " Masukkan kode: " + ColorBold + ColorGreen + code + ColorReset + "\n")
                message.WriteString(ColorCyan + "7." + ColorReset + " Tunggu hingga bot " + ColorBold + ColorGreen + "tersambung" + ColorReset + "\n")
                message.WriteString("\n")
                message.WriteString(ColorGreen + "✅ Bot siap untuk chat private dan grup!" + ColorReset + "\n")
        } else {
                message.WriteString(ColorGreen + "Status: " + ColorReset + ColorBold + code + ColorReset + "\n")
                message.WriteString(ColorGreen + "✅ Bot sedang berjalan untuk chat private dan grup!" + ColorReset + "\n")
        }

        message.WriteString("\n")

        return message.String()
}

func mes(client *whatsmeow.Client, evt Ev) {
        if evt.Message != nil {
                v := evt.Message

                if v.Info.Chat.Server == types.BroadcastServer {
                        features.HandleStoryMessage(client, v)
                        return
                }

                features.HandleAutoPresence(client, v)

                ctx := context.Background()
                botJID := client.Store.ID

                var messageText string
                if v.Message.GetConversation() != "" {
                        messageText = v.Message.GetConversation()
                } else if v.Message.GetExtendedTextMessage() != nil {
                        messageText = v.Message.GetExtendedTextMessage().GetText()
                }

                isSelfMode := utils.IsSelfMessage(client, v.Info.Sender) ||
                        v.Info.Sender.User == botJID.User ||
                        v.Info.Chat.User == botJID.User ||
                        v.Info.IsFromMe

                if utils.IsGroupJID(v.Info.Chat.String()) {
                        utils.CacheGroupParticipantMappings(client, v.Info.Chat)
                }

                if isSelfMode {
                        cmd, args, isCmd := commands.ParseCommand(messageText)
                        if isCmd {
                                switch cmd {
                        case "bot":
                                replyMsg := &waProto.Message{
                                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                Text: proto.String("AKTIF bang"),
                                                ContextInfo: &waProto.ContextInfo{
                                                        StanzaID:    proto.String(v.Info.ID),
                                                        Participant: proto.String(v.Info.Sender.String()),
                                                },
                                        },
                                }

                                _, err := client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                if err != nil {
                                        fmt.Printf("%s⚠️ Failed to send BOT response: %v%s\n", ColorYellow, err, ColorReset)
                                } else {
                                        fmt.Printf("%s✅ Responded to BOT command%s\n", ColorGreen, ColorReset)
                                }
                        case "menu":
                                commands.HandleMenuCommand(client, v.Info.Chat, v.Info.ID, v.Info.Sender)
                        case "info":
                                commands.HandleInfoCommand(client, v.Info.Chat, v.Info.ID, v.Info.Sender)
                        case "ping":
                                commands.HandlePingCommand(client, v.Info.Chat, v.Info.ID, v.Info.Sender)
                        case "online":
                                if args == "on" {
                                        core.UpdateConfig(&[]bool{true}[0], nil, nil, nil, nil, nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("✅ Auto Online DIAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s✅ Auto Online enabled%s\n", ColorGreen, ColorReset)
                                } else if args == "off" {
                                        core.UpdateConfig(&[]bool{false}[0], nil, nil, nil, nil, nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("❌ Auto Online DINONAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s❌ Auto Online disabled%s\n", ColorYellow, ColorReset)
                                }
                        case "typing":
                                if args == "on" {
                                        core.UpdateConfig(nil, &[]bool{true}[0], nil, nil, nil, nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("✅ Auto Typing DIAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s✅ Auto Typing enabled%s\n", ColorGreen, ColorReset)
                                } else if args == "off" {
                                        core.UpdateConfig(nil, &[]bool{false}[0], nil, nil, nil, nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("❌ Auto Typing DINONAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s❌ Auto Typing disabled%s\n", ColorYellow, ColorReset)
                                }
                        case "record":
                                if args == "on" {
                                        core.UpdateConfig(nil, nil, &[]bool{true}[0], nil, nil, nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("✅ Auto Recording DIAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s✅ Auto Recording enabled%s\n", ColorGreen, ColorReset)
                                } else if args == "off" {
                                        core.UpdateConfig(nil, nil, &[]bool{false}[0], nil, nil, nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("❌ Auto Recording DINONAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s❌ Auto Recording disabled%s\n", ColorYellow, ColorReset)
                                }
                        case "readstory":
                                if args == "on" {
                                        core.UpdateConfig(nil, nil, nil, &[]bool{true}[0], nil, nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("✅ Auto Read Story DIAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s✅ Auto Read Story enabled%s\n", ColorGreen, ColorReset)
                                } else if args == "off" {
                                        core.UpdateConfig(nil, nil, nil, &[]bool{false}[0], nil, nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("❌ Auto Read Story DINONAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s❌ Auto Read Story disabled%s\n", ColorYellow, ColorReset)
                                }
                        case "likestory":
                                if args == "on" {
                                        core.UpdateConfig(nil, nil, nil, nil, &[]bool{true}[0], nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("✅ Auto Like Story DIAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s✅ Auto Like Story enabled%s\n", ColorGreen, ColorReset)
                                } else if args == "off" {
                                        core.UpdateConfig(nil, nil, nil, nil, &[]bool{false}[0], nil)
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("❌ Auto Like Story DINONAKTIFKAN"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s❌ Auto Like Story disabled%s\n", ColorYellow, ColorReset)
                                }
                        case "storydelay":
                                if args == "on" {
                                        core.UpdateConfig(nil, nil, nil, nil, nil, &[]bool{true}[0])
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("✅ Story Random Delay DIAKTIFKAN (1-20 detik)"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s✅ Story Random Delay enabled (1-20s)%s\n", ColorGreen, ColorReset)
                                } else if args == "off" {
                                        core.UpdateConfig(nil, nil, nil, nil, nil, &[]bool{false}[0])
                                        replyMsg := &waProto.Message{
                                                ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                        Text: proto.String("❌ Story Random Delay DINONAKTIFKAN (1 detik)"),
                                                        ContextInfo: &waProto.ContextInfo{
                                                                StanzaID:    proto.String(v.Info.ID),
                                                                Participant: proto.String(v.Info.Sender.String()),
                                                        },
                                                },
                                        }
                                        client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                        fmt.Printf("%s❌ Story Random Delay disabled (1s)%s\n", ColorYellow, ColorReset)
                                }
                        case "status":
                                config := core.GetConfig()
                                onlineStatus := "OFF ❌"
                                if config.AutoOnline {
                                        onlineStatus = "ON ✅"
                                }
                                typingStatus := "OFF ❌"
                                if config.AutoTyping {
                                        typingStatus = "ON ✅"
                                }
                                recordStatus := "OFF ❌"
                                if config.AutoRecording {
                                        recordStatus = "ON ✅"
                                }
                                readStoryStatus := "OFF ❌"
                                if config.AutoReadStory {
                                        readStoryStatus = "ON ✅"
                                }
                                likeStoryStatus := "OFF ❌"
                                if config.AutoLikeStory {
                                        likeStoryStatus = "ON ✅"
                                }
                                storyDelayStatus := "Normal (1s) ❌"
                                if config.StoryRandomDelay {
                                        storyDelayStatus = "Random (1-20s) ✅"
                                }
                                statusText := fmt.Sprintf("📊 STATUS FITUR:\n\n🌐 Auto Online: %s\n🖊️ Auto Typing: %s\n🎤 Auto Recording: %s\n👁️ Auto Read Story: %s\n❤️ Auto Like Story: %s\n⏱️ Story Delay: %s", onlineStatus, typingStatus, recordStatus, readStoryStatus, likeStoryStatus, storyDelayStatus)
                                replyMsg := &waProto.Message{
                                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                                                Text: proto.String(statusText),
                                                ContextInfo: &waProto.ContextInfo{
                                                        StanzaID:    proto.String(v.Info.ID),
                                                        Participant: proto.String(v.Info.Sender.String()),
                                                },
                                        },
                                }
                                client.SendMessage(ctx, v.Info.Chat, replyMsg)
                                fmt.Printf("%s📊 Status checked%s\n", ColorCyan, ColorReset)
                        case "jadibot":
                                features.HandleJadibotCommand(client, v.Info.Chat, v.Info.ID, v.Info.Sender, args)
                                fmt.Printf("%s🤖 Jadibot command executed%s\n", ColorCyan, ColorReset)
                        case "listjadibot":
                                features.HandleListJadibotCommand(client, v.Info.Chat, v.Info.ID, v.Info.Sender)
                                fmt.Printf("%s📋 List jadibot command executed%s\n", ColorCyan, ColorReset)
                        case "deljadibot":
                                features.HandleDelJadibotCommand(client, v.Info.Chat, v.Info.ID, v.Info.Sender, args)
                                fmt.Printf("%s🗑️ Delete jadibot command executed%s\n", ColorCyan, ColorReset)
                                }
                        }
                }
        }
}

func isSessionValid(nomor string) bool {
        ctx := context.Background()
        dbFilePath := "Wilykun/bossbot/" + nomor + ".db"

        if _, err := os.Stat(dbFilePath); os.IsNotExist(err) {
                return false
        }

        dbLog := waLog.Stdout("Database", "ERROR", true)
        dbPath := "file:" + dbFilePath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(FULL)&_pragma=wal_autocheckpoint(100)"
        container, err := sqlstore.New(ctx, "sqlite3", dbPath, dbLog)
        if err != nil {
                return false
        }

        deviceStore, err := container.GetFirstDevice(ctx)
        if err != nil {
                return false
        }

        return deviceStore.ID != nil
}

func cleanInvalidSession(nomor string) {
        dbPath := "Wilykun/bossbot/" + nomor + ".db"

        os.Remove(dbPath)
        os.Remove(dbPath + "-shm")
        os.Remove(dbPath + "-wal")

        fmt.Printf("%s⚠️ Session tidak valid, file database dihapus otomatis%s\n", ColorYellow, ColorReset)
}

func checkpointDatabase(nomor string) {
        dbFilePath := "Wilykun/bossbot/" + nomor + ".db"

        if _, err := os.Stat(dbFilePath); os.IsNotExist(err) {
                return
        }

        db, err := sql.Open("sqlite3", "file:"+dbFilePath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)")
        if err != nil {
                fmt.Printf("%s⚠️ Gagal membuka database untuk checkpoint: %v%s\n", ColorYellow, err, ColorReset)
                return
        }
        defer db.Close()

        _, err = db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
        if err != nil {
                fmt.Printf("%s⚠️ Gagal checkpoint database: %v%s\n", ColorYellow, err, ColorReset)
                return
        }

        fmt.Printf("%s✅ Database checkpoint berhasil%s\n", ColorGreen, ColorReset)
}

func getExistingPhoneNumbers() []string {
        entries, err := os.ReadDir("Wilykun/bossbot")
        if err != nil {
                return nil
        }

        var numbers []string
        for _, entry := range entries {
                if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".db") {
                        number := strings.TrimSuffix(entry.Name(), ".db")
                        if number != "" {
                                numbers = append(numbers, number)
                        }
                }
        }
        return numbers
}

func initializeCoreFunctions() {
        core.SetAutoTypingEnabled = features.SetAutoTypingEnabled
        core.SetAutoRecordingEnabled = features.SetAutoRecordingEnabled
        core.SetAutoReadStory = features.SetAutoReadStory
        core.SetAutoLikeStory = features.SetAutoLikeStory
        core.SetStoryRandomDelay = features.SetStoryRandomDelay
}

func main() {
        var nomer string
        var pairingMethod int
        var sessionValid bool = false

        initializeCoreFunctions()
        core.InitConfig()
        commands.BotStartTime = time.Now()
        botStartTime = time.Now()

        fmt.Print(ColorBold + ColorCyan + "\n🤖 WhatsApp Auto-React Bot Starter\n" + ColorReset)
        fmt.Print(ColorYellow + "=" + strings.Repeat("=", 35) + ColorReset + "\n\n")

        nomer = os.Getenv("WHATSAPP_NUMBER")
        if nomer == "" {
                nomer = os.Getenv("NOMOR_BOT")
        }

        if nomer == "" {
                existingNumbers := getExistingPhoneNumbers()
                if len(existingNumbers) == 1 {
                        nomer = existingNumbers[0]
                        fmt.Print(ColorGreen + "✅ Menggunakan session yang ada: " + ColorReset + ColorBold + nomer + ColorReset + "\n")

                        fmt.Print(ColorCyan + "🔍 Mengecek validitas session...\n" + ColorReset)
                        if !isSessionValid(nomer) {
                                cleanInvalidSession(nomer)
                                fmt.Print(ColorYellow + "❌ Session tidak valid, file dihapus!\n" + ColorReset)
                                nomer = ""
                        } else {
                                fmt.Print(ColorGreen + "✅ Session valid!\n" + ColorReset)
                                sessionValid = true
                        }
                } else if len(existingNumbers) > 1 {
                        fmt.Print(ColorYellow + "📱 Ditemukan beberapa session:\n" + ColorReset)
                        for i, num := range existingNumbers {
                                fmt.Printf("%s%d. %s\n%s", ColorCyan, i+1, num, ColorReset)
                        }
                        fmt.Print(ColorGreen + "Pilih nomor (1-" + fmt.Sprintf("%d", len(existingNumbers)) + ") atau ketik nomor baru: " + ColorReset)
                        var choice string
                        fmt.Scanln(&choice)

                        if idx := parseChoice(choice, len(existingNumbers)); idx >= 0 {
                                nomer = existingNumbers[idx]
                                fmt.Print(ColorGreen + "✅ Menggunakan session: " + ColorReset + ColorBold + nomer + ColorReset + "\n")

                                fmt.Print(ColorCyan + "🔍 Mengecek validitas session...\n" + ColorReset)
                                if !isSessionValid(nomer) {
                                        cleanInvalidSession(nomer)
                                        fmt.Print(ColorYellow + "❌ Session tidak valid, file dihapus!\n" + ColorReset)
                                        nomer = ""
                                } else {
                                        fmt.Print(ColorGreen + "✅ Session valid!\n" + ColorReset)
                                        sessionValid = true
                                }
                        } else {
                                nomer = choice
                                fmt.Print(ColorGreen + "✅ Nomor baru: " + ColorReset + ColorBold + nomer + ColorReset + "\n")
                        }
                }

                if nomer == "" {
                        fmt.Print(ColorCyan + "📱 Masukkan nomor WhatsApp (misal: 628xxxxx): " + ColorReset)
                        fmt.Scanln(&nomer)
                        if nomer == "" {
                                fmt.Print(ColorReset + ColorBold + ColorYellow + "❌ Nomor tidak boleh kosong!\n" + ColorReset)
                                os.Exit(1)
                        }
                }
        } else {
                fmt.Print(ColorGreen + "✅ Nomor dari environment: " + ColorReset + ColorBold + nomer + ColorReset + "\n")

                fmt.Print(ColorCyan + "🔍 Mengecek validitas session...\n" + ColorReset)
                if !isSessionValid(nomer) {
                        cleanInvalidSession(nomer)
                        fmt.Print(ColorYellow + "❌ Session tidak valid, file dihapus!\n" + ColorReset)
                        fmt.Print(ColorCyan + "📱 Masukkan nomor WhatsApp (misal: 6289681008411): " + ColorReset)
                        fmt.Scanln(&nomer)
                        if nomer == "" {
                                fmt.Print(ColorReset + ColorBold + ColorYellow + "❌ Nomor tidak boleh kosong!\n" + ColorReset)
                                os.Exit(1)
                        }
                } else {
                        fmt.Print(ColorGreen + "✅ Session valid!\n" + ColorReset)
                        sessionValid = true
                }
        }

        countryInfo := getCountryInfo(nomer)
        fmt.Print(ColorGreen + "✅ Negara: " + ColorReset + ColorBold + countryInfo.CountryName + ColorReset + "\n\n")

        pairingMethodStr := os.Getenv("WHATSAPP_PAIRING_METHOD")
        if pairingMethodStr != "" {
                _, err := fmt.Sscanf(pairingMethodStr, "%d", &pairingMethod)
                if err != nil || (pairingMethod != 1 && pairingMethod != 2) {
                        pairingMethod = 2
                }
                methodName := "QR Code"
                if pairingMethod == 1 {
                        methodName = "Kode Pairing"
                }
                fmt.Print(ColorGreen + "✅ Metode Pairing: " + ColorReset + ColorBold + methodName + ColorReset + "\n\n")
        } else if sessionValid {
                pairingMethod = 2
                fmt.Print(ColorGreen + "✅ Metode Pairing: " + ColorReset + ColorBold + "QR Code (default)" + ColorReset + "\n\n")
        } else {
                fmt.Print(ColorYellow + "Pilih metode pairing:\n" + ColorReset)
                fmt.Print(ColorCyan + "1. Kode Pairing (Manual)\n" + ColorReset)
                fmt.Print(ColorCyan + "2. QR Code (Scan dengan HP)\n" + ColorReset)
                fmt.Print(ColorGreen + "Pilih (1/2): " + ColorReset)
                fmt.Scanln(&pairingMethod)

                if pairingMethod != 1 && pairingMethod != 2 {
                        fmt.Print(ColorReset + ColorBold + ColorYellow + "❌ Pilihan tidak valid! Gunakan 1 atau 2\n" + ColorReset)
                        os.Exit(1)
                }

                methodName := "QR Code"
                if pairingMethod == 1 {
                        methodName = "Kode Pairing"
                }
                fmt.Print(ColorGreen + "✅ Metode dipilih: " + ColorReset + ColorBold + methodName + ColorReset + "\n\n")
        }

        go Connect(nomer, mes, pairingMethod == 2)

        go func() {
                time.Sleep(5 * time.Second)
                fmt.Printf("%s🤖 Memuat jadibot sessions...%s\n", ColorCyan, ColorReset)
                features.GetJadibotManager().LoadExistingSessions()
        }()

        c := make(chan os.Signal, 1)
        signal.Notify(c, os.Interrupt, syscall.SIGTERM)
        <-c

        fmt.Println("\n" + ColorYellow + "⚠️ Menerima signal shutdown, menyimpan data..." + ColorReset)
        checkpointDatabase(nomer)
        fmt.Println(ColorGreen + "✅ Data tersimpan dengan aman!" + ColorReset)
        os.Exit(0)
}
