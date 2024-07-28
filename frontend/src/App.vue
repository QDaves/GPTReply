<template>
  <div class="gpt-logger">
    <header>
      <h1>GPT Reply</h1>
      <button 
        @click="toggleExtension" 
        :class="['toggle-button', { 'enabled': isExtensionEnabled }]"
      >
        {{ isExtensionEnabled ? 'Disable' : 'Enable' }} GPT
      </button>
    </header>
    <main>
      <section v-if="settingsLoaded" class="settings-panel">
        <h2>Settings</h2>
        <div class="settings-grid">
          <div class="setting-card">
            <h3>General</h3>
            <div class="switch-container">
              <label class="switch">
                <input type="checkbox" v-model="settings.RespondToAll" @change="updateSettings" :disabled="!isExtensionEnabled">
                <span class="slider"></span>
              </label>
              <span class="label-text">Ignore Prefix</span>
            </div>
            <div class="input-group" :class="{ 'disabled': settings.RespondToAll || !isExtensionEnabled }">
              <label for="prefix">Prefix:</label>
              <input id="prefix" v-model="settings.Prefix" @change="updateSettings" :disabled="settings.RespondToAll || !isExtensionEnabled" class="short-input">
            </div>
            <div class="input-group">
              <label for="responseDelay">Cooldown Delay:</label>
              <input 
                id="responseDelay" 
                type="number" 
                v-model.number="settings.ResponseDelay" 
                @change="updateSettings" 
                @input="validatePositiveNumber('ResponseDelay')"
                min="0"
                class="short-input" 
                :disabled="!isExtensionEnabled"
              >
            </div>
            <div class="input-group">
              <label for="prevChatCount">Chat History Count:</label>
              <input 
                id="prevChatCount" 
                type="number" 
                v-model.number="settings.PreviousChatCount" 
                @change="updateSettings" 
                @input="validatePositiveNumber('PreviousChatCount')"
                min="0"
                :disabled="!isExtensionEnabled"
              >
            </div>
            <div class="input-group">
              <label for="ignoredUsers">Ignored Users:</label>
              <input id="ignoredUsers" v-model="ignoredUsersInput" @change="updateIgnoredUsers" placeholder="Comma-separated list" class="short-input" :disabled="!isExtensionEnabled">
            </div>
            <div class="input-group">
              <label for="blacklistWords">Blacklisted Words:</label>
              <input id="blacklistWords" v-model="blacklistWordsInput" @change="updateBlacklistWords" placeholder="Comma-separated list" class="short-input" :disabled="!isExtensionEnabled">
            </div>
          </div>
          <div class="setting-card">
            <h3>API Configuration</h3>
            <div class="input-group">
              <label for="apiSelection">API Selection:</label>
              <select id="apiSelection" v-model="settings.UseClaudeAPI" @change="updateSettings" :disabled="!isExtensionEnabled">
                <option :value="false">ChatGPT</option>
                <option :value="true">Claude</option>
              </select>
            </div>
            <div class="input-group">
              <label for="apiKey">API Key:</label>
              <input id="apiKey" v-model="apiKey" type="password" @change="updateSettings" :disabled="!isExtensionEnabled">
            </div>
            <div class="input-group">
              <label for="maxTokens">Max Tokens:</label>
              <input 
                id="maxTokens" 
                type="number" 
                v-model.number="settings.MaxTokens" 
                @change="updateSettings" 
                @input="validatePositiveNumber('MaxTokens')"
                min="0"
                :disabled="!isExtensionEnabled"
              >
            </div>
            <div class="input-group">
              <label for="temperature">Temperature:</label>
              <input 
                id="temperature" 
                type="number" 
                v-model.number="settings.Temperature" 
                @change="updateSettings" 
                @input="validateTemperature"
                step="0.1" 
                min="0" 
                max="1" 
                :disabled="!isExtensionEnabled"
              >
            </div>
            <div class="input-group">
              <label for="model">Model:</label>
              <select id="model" v-model="selectedModel" @change="updateSettings" :disabled="!isExtensionEnabled">
                <option v-for="model in settings.UseClaudeAPI ? claudeModels : openAIModels" :key="model" :value="model">{{ model }}</option>
              </select>
            </div>
          </div>
          <div class="setting-card full-width">
            <h3>Chat Instructions</h3>
            <textarea v-model="settings.ChatInstructions" @change="updateSettings" rows="4" spellcheck="false" :disabled="!isExtensionEnabled"></textarea>
          </div>
        </div>
      </section>
      <section v-else class="loading">Loading settings...</section>
      <section class="chat-log">
        <h2>Chat Log</h2>
        <div class="log-container" ref="logBox">
          <div v-for="(entry, index) in chatLog" :key="index" class="log-entry">
            <span class="timestamp">{{ formatTimestamp(entry.Time) }}</span>
            <span class="username">{{ entry.Username }}:</span>
            <span class="message">{{ entry.Message }}</span>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script>
import { GetSettings, UpdateSettings, GetChatLog, SaveSettings, ToggleExtension, GetExtensionState } from '../wailsjs/go/main/App'

export default {
  data() {
    return {
      settings: {
        Prefix: '',
        ResponseDelay: 0,
        IgnoredUsers: [],
        BlacklistWords: [],
        ChatInstructions: '',
        UseClaudeAPI: false,
        ChatGPTApiKey: '',
        ClaudeApiKey: '',
        RespondToAll: false,
        PreviousChatCount: 0,
        MaxTokens: 100,
        Temperature: 0.7,
        OpenAIModel: '',
        ClaudeModel: ''
      },
      ignoredUsersInput: '',
      blacklistWordsInput: '',
      chatLog: [],
      openAIModels: ["gpt-3.5-turbo", "gpt-4", "gpt-4o-mini", "gpt-4o", "gpt-4-turbo", "gpt-3.5-turbo-0125", "gpt-4-0613", "gpt-4-32k-0613", "gpt-3.5-turbo-0613", "gpt-3.5-turbo-16k", "gpt-4-vision-preview"],
      claudeModels: ["claude-3-opus-20240229", "claude-3-sonnet-20240229", "claude-3-haiku-20240307", "claude-3-5-sonnet-20240620"],
      settingsLoaded: false,
      settingsSaved: false,
      isExtensionEnabled: false
    }
  },
  computed: {
    apiKey: {
      get() {
        return this.settings.UseClaudeAPI ? this.settings.ClaudeApiKey : this.settings.ChatGPTApiKey
      },
      set(value) {
        if (this.settings.UseClaudeAPI) {
          this.settings.ClaudeApiKey = value
        } else {
          this.settings.ChatGPTApiKey = value
        }
        this.saveSettings()
      }
    },
    selectedModel: {
      get() {
        return this.settings.UseClaudeAPI ? this.settings.ClaudeModel : this.settings.OpenAIModel
      },
      set(value) {
        if (this.settings.UseClaudeAPI) {
          this.settings.ClaudeModel = value
        } else {
          this.settings.OpenAIModel = value
        }
        this.saveSettings()
      }
    }
  },
  methods: {
    async fetchSettings() {
      try {
        const loadedSettings = await GetSettings()
        this.settings = loadedSettings
        this.ignoredUsersInput = this.settings.IgnoredUsers ? this.settings.IgnoredUsers.join(', ') : ''
        this.blacklistWordsInput = this.settings.BlacklistWords ? this.settings.BlacklistWords.join(', ') : ''
        this.settingsLoaded = true
      } catch (error) {
      }
    },
    updateIgnoredUsers() {
      this.settings.IgnoredUsers = this.ignoredUsersInput.split(',').map(u => u.trim()).filter(u => u)
      this.saveSettings()
    },
    updateBlacklistWords() {
      this.settings.BlacklistWords = this.blacklistWordsInput.split(',').map(w => w.trim()).filter(w => w)
      this.saveSettings()
    },
    formatTimestamp(timestamp) {
      const date = new Date(timestamp)
      return date.toLocaleTimeString()
    },
    async fetchChatLog() {
      try {
        this.chatLog = await GetChatLog()
        this.$nextTick(() => {
          this.scrollToBottom()
        })
      } catch (error) {
      }
    },
    scrollToBottom() {
      const logBox = this.$refs.logBox
      if (logBox) {
        logBox.scrollTop = logBox.scrollHeight
      }
    },
    async saveSettings() {
      try {
        await SaveSettings(this.settings)
        this.settingsSaved = true
        setTimeout(() => {
          this.settingsSaved = false
        }, 3000)
      } catch (error) {
      }
    },
    async toggleExtension() {
      try {
        const newState = await ToggleExtension()
        this.isExtensionEnabled = newState
      } catch (error) {
      }
    },
    async fetchExtensionState() {
      try {
        this.isExtensionEnabled = await GetExtensionState()
      } catch (error) {
      }
    },
    validatePositiveNumber(field) {
      if (this.settings[field] < 0) {
        this.settings[field] = 0;
      }
    },
    validateTemperature() {
      if (this.settings.Temperature < 0) {
        this.settings.Temperature = 0;
      } else if (this.settings.Temperature > 1) {
        this.settings.Temperature = 1;
      }
    }
  },
  async created() {
    await this.fetchSettings()
    await this.fetchExtensionState()
    this.fetchChatLog()
  },
  mounted() {
    window.runtime.EventsOn("logUpdate", (message) => {
      this.fetchChatLog()
    })
  },
  watch: {
    settings: {
      handler(newSettings) {
        this.saveSettings()
      },
      deep: true
    }
  }
}
</script>

<style scoped>
.gpt-logger {
  font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
  color: #e0e0e0;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

header {
  background-color: #2c2c2c;
  padding: 20px;
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

h1, h2, h3 {
  color: #4CAF50;
  margin: 0;
}

h1 {
  font-size: 24px;
}

h2 {
  font-size: 20px;
  margin-bottom: 20px;
}

h3 {
  font-size: 18px;
  margin-bottom: 15px;
}

main {
  flex-grow: 1;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.settings-panel {
  background-color: #2c2c2c;
  border-radius: 8px;
  padding: 20px;
  box-shadow: 0 4px 6px rgba(0,0,0,0.1);
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 20px;
  margin-bottom: 20px;
}

.setting-card {
  background-color: #383838;
  padding: 20px;
  border-radius: 8px;
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}

.setting-card.full-width textarea {
  width: 97%;
  min-height: 100px;
  padding: 10px;
  font-size: 15px;
  line-height: 1;
  resize: vertical;
}

.full-width {
  grid-column: 1 / -1;
}

.input-group {
  margin-bottom: 15px;
}

label {
  display: block;
  margin-bottom: 5px;
  color: #b0b0b0;
}

input[type="text"],
input[type="number"],
input[type="password"],
textarea,
select {
  width: 100%;
  padding: 6px 12px;
  background-color: #2c2c2c;
  border: 1px solid #4c4c4c;
  border-radius: 4px;
  color: #e0e0e0;
  font-size: 14px;
}

input[type="number"],
#apiKey,
#prefix,
#responseDelay,
#ignoredUsers,
#blacklistWords {
  width: 250px;
  padding: 6px 12px;
  background-color: #2c2c2c;
  border: 1px solid #4c4c4c;
  border-radius: 4px;
  color: #e0e0e0;
  font-size: 16px;
}

select {
  appearance: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' fill='%23e0e0e0'%3E%3Cpath d='M3 4h6L6 8z'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 12px center;
  padding-right: 30px;
}

textarea {
  resize: vertical;
  min-height: 100px;
}

.switch-container {
  display: flex;
  align-items: center;
  margin-bottom: 15px;
}

.switch {
  position: relative;
  display: inline-block;
  width: 60px;
  height: 34px;
  margin-right: 10px;
}

.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: #4c4c4c;
  transition: .4s;
  border-radius: 34px;
}

.slider:before {
  position: absolute;
  content: "";
  height: 26px;
  width: 26px;
  left: 4px;
  bottom: 4px;
  background-color: #e0e0e0;
  transition: .4s;
  border-radius: 50%;
}

input:checked + .slider {
  background-color: #4CAF50;
}

input:checked + .slider:before {
  transform: translateX(26px);
}

.label-text {
  line-height: 34px;
}

.chat-log {
  background-color: #2c2c2c;
  border-radius: 8px;
  padding: 20px;
  box-shadow: 0 4px 6px rgba(0,0,0,0.1);
}

.log-container {
  height: 300px;
  overflow-y: auto;
  background-color: #383838;
  border-radius: 4px;
  padding: 10px;
  scrollbar-width: none; 
  -ms-overflow-style: none;
}

.log-container::-webkit-scrollbar {
  width: 0;
  height: 0;
  display: none; 
}

.log-entry {
  margin-bottom: 10px;
  line-height: 1.4;
}

.timestamp {
  color: #888;
  margin-right: 10px;
  font-size: 12px;
}

.username {
  color: #4CAF50;
  font-weight: bold;
  margin-right: 5px;
}

.message {
  color: #e0e0e0;
}

.loading {
  text-align: center;
  font-size: 18px;
  color: #888;
}

.disabled {
  opacity: 0.5;
  pointer-events: none;
}

.toggle-button {
  padding: 10px 20px;
  font-size: 16px;
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.3s, color 0.3s;
  border: none;
  outline: none;
}

.toggle-button.enabled {
  background-color: #4CAF50;
  color: white;
}

.toggle-button:not(.enabled) {
  background-color: #f44336;
  color: white;
}

.toggle-button:hover {
  opacity: 0.9;
}
</style>