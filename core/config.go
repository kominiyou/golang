package core

import (
	"encoding/json"
	"os"
	"sync"
)

type BotConfig struct {
	AutoOnline       bool `json:"auto_online"`
	AutoTyping       bool `json:"auto_typing"`
	AutoRecording    bool `json:"auto_recording"`
	AutoReadStory    bool `json:"auto_read_story"`
	AutoLikeStory    bool `json:"auto_like_story"`
	StoryRandomDelay bool `json:"story_random_delay"`
}

var (
	currentConfig = BotConfig{
		AutoOnline:       true,
		AutoTyping:       false,
		AutoRecording:    false,
		AutoReadStory:    true,
		AutoLikeStory:    true,
		StoryRandomDelay: true,
	}
	configMutex sync.RWMutex
	stateFile   = "Wilykun/settings.dat"
)

var (
	AutoTypingEnabled    func() bool
	AutoRecordingEnabled func() bool
	AutoReadStoryFunc    func() bool
	AutoLikeStoryFunc    func() bool
	StoryRandomDelayFunc func() bool

	SetAutoTypingEnabled    func(bool)
	SetAutoRecordingEnabled func(bool)
	SetAutoReadStory        func(bool)
	SetAutoLikeStory        func(bool)
	SetStoryRandomDelay     func(bool)
)

func InitConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()

	data, err := os.ReadFile(stateFile)
	if err == nil {
		var loaded map[string]interface{}
		if json.Unmarshal(data, &loaded) == nil {
			if val, ok := loaded["auto_online"].(bool); ok {
				currentConfig.AutoOnline = val
			}
			if val, ok := loaded["auto_typing"].(bool); ok {
				currentConfig.AutoTyping = val
			}
			if val, ok := loaded["auto_recording"].(bool); ok {
				currentConfig.AutoRecording = val
			}
			if val, ok := loaded["auto_read_story"].(bool); ok {
				currentConfig.AutoReadStory = val
			}
			if val, ok := loaded["auto_like_story"].(bool); ok {
				currentConfig.AutoLikeStory = val
			}
			if val, ok := loaded["story_random_delay"].(bool); ok {
				currentConfig.StoryRandomDelay = val
			}
		}
	}

	if SetAutoTypingEnabled != nil {
		SetAutoTypingEnabled(currentConfig.AutoTyping)
	}
	if SetAutoRecordingEnabled != nil {
		SetAutoRecordingEnabled(currentConfig.AutoRecording)
	}
	if SetAutoReadStory != nil {
		SetAutoReadStory(currentConfig.AutoReadStory)
	}
	if SetAutoLikeStory != nil {
		SetAutoLikeStory(currentConfig.AutoLikeStory)
	}
	if SetStoryRandomDelay != nil {
		SetStoryRandomDelay(currentConfig.StoryRandomDelay)
	}
}

func saveState() {
	data, err := json.Marshal(currentConfig)
	if err != nil {
		return
	}
	os.WriteFile(stateFile, data, 0644)
}

func GetConfig() BotConfig {
	configMutex.RLock()
	defer configMutex.RUnlock()

	return currentConfig
}

func UpdateConfig(autoOnline, autoTyping, autoRecording, autoReadStory, autoLikeStory, storyRandomDelay *bool) {
	configMutex.Lock()
	defer configMutex.Unlock()

	if autoOnline != nil {
		currentConfig.AutoOnline = *autoOnline
	}
	if autoTyping != nil {
		currentConfig.AutoTyping = *autoTyping
		if SetAutoTypingEnabled != nil {
			SetAutoTypingEnabled(*autoTyping)
		}
	}
	if autoRecording != nil {
		currentConfig.AutoRecording = *autoRecording
		if SetAutoRecordingEnabled != nil {
			SetAutoRecordingEnabled(*autoRecording)
		}
	}
	if autoReadStory != nil {
		currentConfig.AutoReadStory = *autoReadStory
		if SetAutoReadStory != nil {
			SetAutoReadStory(*autoReadStory)
		}
	}
	if autoLikeStory != nil {
		currentConfig.AutoLikeStory = *autoLikeStory
		if SetAutoLikeStory != nil {
			SetAutoLikeStory(*autoLikeStory)
		}
	}
	if storyRandomDelay != nil {
		currentConfig.StoryRandomDelay = *storyRandomDelay
		if SetStoryRandomDelay != nil {
			SetStoryRandomDelay(*storyRandomDelay)
		}
	}

	saveState()
}
