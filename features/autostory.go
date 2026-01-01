package features

import (
        "context"
        "fmt"
        "math/rand"
        "sync"
        "sync/atomic"
        "time"

        "go.mau.fi/whatsmeow"
        "go.mau.fi/whatsmeow/types"
        "go.mau.fi/whatsmeow/types/events"

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
)

var (
        autoReadStory    atomic.Bool
        autoLikeStory    atomic.Bool
        storyRandomDelay atomic.Bool
        storyEmojis      = []string{
                "🔥", "👍", "😂", "🎉", "💯", "⚡", "✨", "🙏", "👏", "💪",
                "🤩", "😎", "🤙", "🌟", "🚀", "🗿", "🥳", "😊", "🤗", "😜",
                "🌈", "🌸", "🌺", "🌻", "🌹", "🌷", "🍀", "🎊", "🎈", "🎁",
                "🏆", "🥇", "🎯", "💎", "👑", "🔮", "🎵", "🎶", "🎤", "🎧",
                "📸", "🎬", "🌙", "⭐", "🌠", "☀️", "🌊", "🏝️", "🦋", "🐝",
                "🦄", "🐬", "🦅", "🦁", "🐯", "🦊", "🐻", "🐼", "🐨", "🐰",
                "🍕", "🍔", "🍟", "🍩", "🍪", "🍫", "🍰", "🎂", "🍿", "☕",
                "🥂", "🍾", "🍷", "🥤", "🧋", "🍉", "🍓", "🍑", "🍒", "🥑",
                "🎸", "🎹", "🎺", "🎻", "🥁", "🎲", "🎮", "🎯", "🎪", "🎭",
                "🚗", "🚕", "🚙", "🚌", "🚎", "🏎️", "🚓", "🚑", "🚒", "✈️",
        }
        processedStory = make(map[string]time.Time)
        storyMutex     sync.Mutex
        storyCooldown  = 3 * time.Second
)

func init() {
        autoReadStory.Store(true)
        autoLikeStory.Store(true)
        storyRandomDelay.Store(true)
}

func GetAutoReadStory() bool {
        return autoReadStory.Load()
}

func SetAutoReadStory(val bool) {
        autoReadStory.Store(val)
}

func GetAutoLikeStory() bool {
        return autoLikeStory.Load()
}

func SetAutoLikeStory(val bool) {
        autoLikeStory.Store(val)
}

func GetStoryRandomDelay() bool {
        return storyRandomDelay.Load()
}

func SetStoryRandomDelay(val bool) {
        storyRandomDelay.Store(val)
}

func getRandomEmoji() string {
        return storyEmojis[rand.Intn(len(storyEmojis))]
}

func getStoryDelay() time.Duration {
        if GetStoryRandomDelay() {
                delaySeconds := rand.Intn(20) + 1
                return time.Duration(delaySeconds) * time.Second
        }
        return 1 * time.Second
}

func formatPhoneNumber(number string) string {
        if len(number) > 8 {
                return number[:4] + "****" + number[len(number)-3:]
        }
        return number
}

func getContactName(client *whatsmeow.Client, jid types.JID) string {
        ctx := context.Background()
        contact, err := client.Store.Contacts.GetContact(ctx, jid)
        if err == nil && contact.FullName != "" {
                return contact.FullName
        }
        if err == nil && contact.PushName != "" {
                return contact.PushName
        }
        return ""
}

func isStoryDeleted(msg *events.Message) bool {
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

func HandleStoryMessage(client *whatsmeow.Client, msg *events.Message) {
        if !GetAutoReadStory() && !GetAutoLikeStory() {
                return
        }

        if msg.Info.Chat.Server != types.BroadcastServer {
                return
        }

        if isStoryDeleted(msg) {
                return
        }

        senderInfo := utils.GetAccurateSenderInfo(client, msg.Info.Sender, msg.Info.Chat, msg.Info.IsFromMe)

        botJID := client.Store.ID
        if utils.IsSelfMessage(client, msg.Info.Sender) || msg.Info.Sender.User == botJID.User {
                return
        }

        phoneNumber := utils.ExtractPhoneFromJID(msg.Info.Sender)
        if phoneNumber == "" && utils.IsPNUser(senderInfo.ID) {
                if parsed, err := types.ParseJID(senderInfo.ID); err == nil {
                        phoneNumber = utils.ExtractPhoneFromJID(parsed)
                }
        }
        if phoneNumber == "" {
                phoneNumber = msg.Info.Sender.User
        }

        storyKey := fmt.Sprintf("%s_%s", msg.Info.ID, phoneNumber)
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

        go cleanupOldStories()

        go processStory(client, msg, phoneNumber, senderInfo)
}

func processStory(client *whatsmeow.Client, msg *events.Message, phoneNumber string, senderInfo utils.SenderInfo) {
        ctx := context.Background()
        senderJID := msg.Info.Sender

        displayName := senderInfo.Name
        if displayName == "" {
                displayName = formatPhoneNumber(phoneNumber)
        }

        emoji := ""
        reactionSuccess := false

        if GetAutoLikeStory() {
                emoji = getRandomEmoji()
        }

        delay := getStoryDelay()
        time.Sleep(delay)

        if GetAutoReadStory() {
                client.MarkRead(ctx, []types.MessageID{msg.Info.ID}, msg.Info.Timestamp, msg.Info.Chat, senderJID)
        }

        if GetAutoLikeStory() && emoji != "" {
                _, err := client.SendMessage(ctx, msg.Info.Chat, client.BuildReaction(msg.Info.Chat, senderJID, msg.Info.ID, emoji))
                if err == nil {
                        reactionSuccess = true
                }
        }

        if GetAutoReadStory() || reactionSuccess {
                loc, _ := time.LoadLocation("Asia/Jakarta")
                now := time.Now().In(loc)
                months := []string{
                        "Januari", "Februari", "Maret", "April", "Mei", "Juni",
                        "Juli", "Agustus", "September", "Oktober", "November", "Desember",
                }
                days := []string{
                        "Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu",
                }
                timeStr := fmt.Sprintf("%02d:%02d:%02d WIB", now.Hour(), now.Minute(), now.Second())
                dateStr := fmt.Sprintf("%s, %d %s %d", days[now.Weekday()], now.Day(), months[now.Month()-1], now.Year())

                greeting := "Selamat Pagi"
                hour := now.Hour()
                if hour >= 12 && hour < 15 {
                        greeting = "Selamat Siang"
                } else if hour >= 15 && hour < 18 {
                        greeting = "Selamat Sore"
                } else if hour >= 18 || hour < 4 {
                        greeting = "Selamat Malam"
                }

                delayMode := "1 Detik"
                if GetStoryRandomDelay() {
                        delayMode = fmt.Sprintf("%d Detik (Random)", int(delay.Seconds()))
                }

                reactionStr := "-"
                if reactionSuccess {
                        reactionStr = emoji
                }

                fmt.Printf("%s├══════════════════════════════════┤%s\n", ColorCyan, ColorReset)
                fmt.Printf("%s│%s » Status      : %sAktif ✓%s\n", ColorCyan, ColorReset, ColorGreen, ColorReset)
                fmt.Printf("%s│%s » Tanggal     : %s%s%s\n", ColorCyan, ColorReset, ColorYellow, dateStr, ColorReset)
                fmt.Printf("%s│%s » Selamat     : %s%s%s\n", ColorCyan, ColorReset, ColorMagenta, greeting, ColorReset)
                fmt.Printf("%s│%s » Waktu       : %s%s%s\n", ColorCyan, ColorReset, ColorBlue, timeStr, ColorReset)
                fmt.Printf("%s│%s » Nama        : %s%s%s\n", ColorCyan, ColorReset, ColorGreen, displayName, ColorReset)
                fmt.Printf("%s│%s » View Delay  : %s%s%s\n", ColorCyan, ColorReset, ColorCyan, delayMode, ColorReset)
                fmt.Printf("%s│%s » Reaksi      : %s%s%s\n", ColorCyan, ColorReset, ColorYellow, reactionStr, ColorReset)
                fmt.Printf("%s└───···%s\n", ColorCyan, ColorReset)
        }
}

func cleanupOldStories() {
        storyMutex.Lock()
        defer storyMutex.Unlock()

        if len(processedStory) > 1000 {
                now := time.Now()
                for key, lastTime := range processedStory {
                        if now.Sub(lastTime) > 5*time.Minute {
                                delete(processedStory, key)
                        }
                }
                if len(processedStory) > 1000 {
                        processedStory = make(map[string]time.Time)
                }
        }
}
