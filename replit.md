# WhatsApp Auto-React Bot

Bot WhatsApp dengan fitur auto-presence, auto-story reaction, dan multi-session jadibot.

## Overview

Bot ini dibangun menggunakan Go dan library whatsmeow untuk koneksi WhatsApp. Fitur utama termasuk auto-read story, auto-reaction, dan sistem jadibot yang memungkinkan orang lain terhubung ke bot dengan fitur yang sama.

## User Preferences

Preferred communication style: Simple, everyday language (Bahasa Indonesia).

## System Architecture

### Backend Architecture
- **Language**: Go (Golang)
- **WhatsApp Library**: go.mau.fi/whatsmeow
- **Database**: SQLite (per session)

### Data Storage
- **Main Session**: `Wilykun/<nomor>.db`
- **Jadibot Sessions**: `Wilykun/jadibot/<nomor>.db`
- **Config**: `Wilykun/settings.dat`

## Key Features

### 1. Auto Presence
- Auto Online
- Auto Typing
- Auto Recording

### 2. Auto Story
- Auto Read Story
- Auto Like Story (dengan emoji random)
- Story Random Delay (1-20 detik)

### 3. Jadibot System
Sistem multi-session yang memungkinkan user lain terhubung ke bot dengan pairing code.

**Commands:**
- `jadibot 6289xxxxxxx` - Daftar sebagai jadibot (dapat pairing code)
- `listjadibot` - Lihat daftar jadibot aktif
- `deljadibot 6289xxxxxxx` - Hapus jadibot

**Fitur Jadibot:**
- Auto Read Story
- Auto Reaction Story
- Auto Reconnect
- Session tersimpan di folder terpisah

## File Structure

```
.
├── main.go                # Entry point, command handlers
├── go.mod                 # Go module file
├── go.sum                 # Go dependencies
├── core/
│   └── config.go          # Bot configuration management
├── features/
│   ├── autopresence.go    # Auto typing/recording features
│   ├── autostory.go       # Auto story read/reaction
│   └── jadibot.go         # Multi-session jadibot management
├── commands/
│   ├── parser.go          # Command parser (multi-prefix support)
│   ├── menu.go            # Menu command handler
│   ├── info.go            # Info command handler
│   └── ping.go            # Ping command handler
├── utils/
│   └── lid_resolver.go    # LID to Phone Number resolution system
└── Wilykun/
    ├── <nomor>.db         # Main session database
    ├── settings.dat       # Bot settings
    └── jadibot/           # Jadibot session databases
```

## LID Resolution System

WhatsApp uses two identifier formats:
- **Phone Number JID (PN)**: `6289xxxxxxx@s.whatsapp.net` - traditional format
- **Local Identifier (LID)**: `xxxxxxxxxxxxxx:xx@lid` - newer format for privacy

The `lid_resolver.go` file handles mapping between these formats:
- Uses whatsmeow's built-in `store.LIDs` interface for persistence
- Prioritizes `participant.PhoneNumber` over `participant.JID` in group resolution
- Proactively caches mappings from all joined groups on connection
- Provides `IsSelfMessage()` function for accurate self-message detection

## Commands

### Command Syntax

Commands sekarang support **multi-prefix** + **tanpa prefix**:
- Prefix support: `.`, `!`, `-`, `/`
- Tanpa prefix: Juga work untuk command yang umum digunakan

**Examples:**
- `.bot` ✅
- `!bot` ✅
- `-bot` ✅
- `/bot` ✅
- `bot` ✅ (tanpa prefix)

### General
- `bot` - Cek bot aktif
- `menu` - Lihat menu lengkap
- `info` - Info bot dan sistem
- `ping` - Cek response time
- `status` - Lihat status semua fitur

### Auto Presence
- `online on/off` - Auto online
- `typing on/off` - Auto typing
- `record on/off` - Auto recording

### Auto Story
- `readstory on/off` - Auto read story
- `likestory on/off` - Auto like story
- `storydelay on/off` - Random delay (1-20s)

### Jadibot
- `jadibot 6289xxx` - Daftar jadibot
- `listjadibot` - List jadibot
- `deljadibot 6289xxx` - Hapus jadibot

## External Dependencies

### Go Packages
- go.mau.fi/whatsmeow - WhatsApp Web API
- github.com/ncruces/go-sqlite3 - SQLite driver
- github.com/mdp/qrterminal/v3 - QR code terminal
- github.com/nyaruka/phonenumbers - Phone number parsing
- google.golang.org/protobuf - Protocol buffers

## Recent Changes

### December 21, 2025 (Privacy & Multi-Prefix Update)
- **Privacy Enhancement - Hapus tampilan sensitif data** ✅:
  - Hapus ID (JID format: `6281345093433:32@s.whatsapp.net`) dari output
  - Hapus LID (Local Identifier: `-`) dari output
  - Hapus Nomor masked (`6281****433`) dari output
  - Berlaku untuk: Bot Utama story output + Jadibot story output
  - **Output sekarang lebih clean & aman:**
    - Tampil: Status, Tanggal, Greeting, Waktu, Nama, View Delay, Reaksi
    - Tidak tampil: ID, LID, Nomor (privacy protected)

- **Multi-Prefix Command Support IMPLEMENTED** ✅:
  - Created `commands/parser.go` dengan flexible command parser
  - Support prefix: `.`, `!`, `-`, `/`
  - Support tanpa prefix untuk common commands
  - Refactored main.go command handling dari if-else-if chain ke switch statement
  - ParseCommand() returns (cmd, args, isCommand)
  - Lebih clean, maintainable, dan mudah tambah command baru

- **Emoji list diupdate - Hapus love/heart/kiss emoji**:
  - Hapus: ❤️, 💖, 💕, 💗, 💓, 💞, 💝, 🧡, 💛, 💚, 💙, 💜, 🖤, 🤍, 🤎, ❣️, 💌, 💋, 😍, 🥰, 😘, 🤗 (22 emoji)
  - Tambah: 🎸, 🎹, 🎺, 🎻, 🥁, 🎲, 🎮, 🎪, 🎭, 🚗, 🚕, 🚙, 🚌, 🚎, 🏎️, 🚓, 🚑, 🚒, ✈️, 😜 (lebih variatif)
  - Total emoji sekarang: 80 emoji (hapus 22, tambah 20)
  - Berlaku untuk: Bot Utama + Semua Jadibot User

- **CRITICAL FIX: Jadibot Reaction feature tidak aktif**:
  - **Bug**: `processJadibotStory()` tidak respect flag `autoReadStory` dan `autoLikeStory`
  - **Masalah**: Reaction SELALU jalan meski fitur off, emoji kadang hilang, read dan reaction tidak konsisten
  - **Solusi**: 
    - Tambah `if GetAutoLikeStory()` sebelum generate emoji
    - Tambah `if GetAutoReadStory()` sebelum MarkRead
    - Tambah `if GetAutoLikeStory() && emoji != ""` sebelum SendMessage reaction
    - Output sekarang hanya tampil jika ada action yang sukses
  - **Hasil**: Read + Reaction sekarang berjalan bersamaan dengan benar, emoji tidak hilang

- **Fixed device not showing as active immediately after connection**:
  - Added multiple aggressive presence updates (3x dengan 300ms delay) setelah event Connected
  - Device sekarang langsung muncul sebagai "Online" di "Perangkat Tertaut" tanpa perlu restart
  
- **Added timeout notification to WhatsApp**:
  - Saat pairing code expired (timeout 3 menit), bot sekarang mengirim notif ke owner
  - Notif berisi nomor jadibot dan instruksi untuk coba lagi
  
- **Added connection success notification**:
  - Saat jadibot berhasil terhubung, owner menerima konfirmasi via WhatsApp
  - Notif menyatakan status online dan fitur sudah berjalan

### December 3, 2025 (Update 2)
- Added comprehensive jadibot validation and auto-cleanup system
- **LoadExistingSessions** now auto-deletes invalid database files when:
  - Database fails to load
  - Device store fails to get
  - Device ID is null
  - Connection fails after startup
- **Health Check System** (runs every 60 seconds):
  - Validates all active jadibot sessions
  - Uses snapshot pattern for thread-safe iteration
  - Tracks fail count per session (max 10 consecutive fails before deletion)
  - Auto-resets fail count after 5 minutes of no failures
  - Coordinates with reconnect to avoid race conditions
  - Cleans up orphaned database files
- **Improved Reconnect Logic**:
  - Maximum 5 retry attempts with progressive backoff (5s, 10s, 20s, 30s, 60s)
  - Uses Reconnecting flag to prevent concurrent reconnection attempts
  - Auto-removes session after all retries fail
  - Proper thread-safe field updates

### December 3, 2025
- Added comprehensive LID (Local Identifier) to Phone Number resolution system
- Created `lid_resolver.go` with proper mapping functions
- Prioritizes `participant.PhoneNumber` over `participant.JID` in group resolution
- Proactive caching of all joined groups' participant mappings on connection
- Improved self-message detection using `IsSelfMessage()` function
- Auto-story now displays accurate LID and phone number information

### November 26, 2025
- Added jadibot system for multi-session support
- Users can connect via `jadibot` command with pairing code
- Each jadibot gets auto-read story and auto-reaction features
- Jadibot sessions stored in `Wilykun/jadibot/` folder
- Added `listjadibot` and `deljadibot` commands for management
- Updated menu to include jadibot commands
