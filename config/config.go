package config

import (
	"errors"
	"fmt"
	"krillin-ai/log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
	"go.uber.org/zap"
)

var ConfigBackup Config // 用于在开始任务之前，检测配置是否更新，更新后要重启服务端

type App struct {
	SegmentDuration       int      `toml:"segment_duration"`
	TranscribeParallelNum int      `toml:"transcribe_parallel_num"`
	TranslateParallelNum  int      `toml:"translate_parallel_num"`
	TranscribeMaxAttempts int      `toml:"transcribe_max_attempts"`
	TranslateMaxAttempts  int      `toml:"translate_max_attempts"`
	MaxSentenceLength     int      `toml:"max_sentence_length"`
	Proxy                 string   `toml:"proxy"`
	ParsedProxy           *url.URL `toml:"-"`
}

type Server struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type OpenaiCompatibleConfig struct {
	BaseUrl      string `toml:"base_url"`
	ApiKey       string `toml:"api_key"`
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt"`
}

type LocalModelConfig struct {
	Model string `toml:"model"`
}

type AliyunSpeechConfig struct {
	AccessKeyId     string `toml:"access_key_id"`
	AccessKeySecret string `toml:"access_key_secret"`
	AppKey          string `toml:"app_key"`
}

type AliyunOssConfig struct {
	AccessKeyId     string `toml:"access_key_id"`
	AccessKeySecret string `toml:"access_key_secret"`
	Bucket          string `toml:"bucket"`
}

type AliyunTranscribeConfig struct {
	Oss    AliyunOssConfig    `toml:"oss"`
	Speech AliyunSpeechConfig `toml:"speech"`
}

type Transcribe struct {
	Provider              string                 `toml:"provider"`
	EnableGpuAcceleration bool                   `toml:"enable_gpu_acceleration"`
	Openai                OpenaiCompatibleConfig `toml:"openai"`
	Fasterwhisper         LocalModelConfig       `toml:"fasterwhisper"`
	Whisperkit            LocalModelConfig       `toml:"whisperkit"`
	Whispercpp            LocalModelConfig       `toml:"whispercpp"`
	Aliyun                AliyunTranscribeConfig `toml:"aliyun"`
}

type AliyunTtsConfig struct {
	Oss    AliyunOssConfig    `toml:"oss"`
	Speech AliyunSpeechConfig `toml:"speech"`
}

type Tts struct {
	Provider string                 `toml:"provider"`
	Openai   OpenaiCompatibleConfig `toml:"openai"`
	Aliyun   AliyunTtsConfig        `toml:"aliyun"`
}

type OpenAiWhisper struct {
	BaseUrl string `toml:"base_url"`
	ApiKey  string `toml:"api_key"`
}

type Config struct {
	App        App                    `toml:"app"`
	Server     Server                 `toml:"server"`
	Llm        OpenaiCompatibleConfig `toml:"llm"`
	Transcribe Transcribe             `toml:"transcribe"`
	Tts        Tts                    `toml:"tts"`
}

var Conf = Config{
	App: App{
		SegmentDuration:       5,
		TranslateParallelNum:  3,
		TranscribeParallelNum: 1,
		TranscribeMaxAttempts: 3,
		TranslateMaxAttempts:  3,
		MaxSentenceLength:     70,
	},
	Server: Server{
		Host: "127.0.0.1",
		Port: 8888,
	},
	Llm: OpenaiCompatibleConfig{
		Model: "gpt-4o-mini",
	},
	Transcribe: Transcribe{
		Provider:              "openai",
		EnableGpuAcceleration: false, // 默认不开启GPU加速
		Openai: OpenaiCompatibleConfig{
			Model: "whisper-1",
		},
		Fasterwhisper: LocalModelConfig{
			Model: "large-v3",
		},
		Whisperkit: LocalModelConfig{
			Model: "large-v3",
		},
		Whispercpp: LocalModelConfig{
			Model: "large-v3",
		},
	},
	Tts: Tts{
		Provider: "openai",
		Openai: OpenaiCompatibleConfig{
			Model: "gpt-4o-mini-tts",
		},
	},
}

// 检查必要的配置是否完整
func validateConfig() error {
	// 检查转写服务提供商配置
	switch Conf.Transcribe.Provider {
	case "openai":
		if Conf.Transcribe.Openai.ApiKey == "" {
			return errors.New("使用OpenAI转录服务需要配置 OpenAI API Key")
		}
	case "fasterwhisper":
		if Conf.Transcribe.Fasterwhisper.Model == "" {
			return errors.New("检测到开启了fasterwhisper，但未配置模型名，请在配置文件中设置 transcribe.fasterwhisper.model")
		}
	case "whisperkit":
		if runtime.GOOS != "darwin" {
			log.GetLogger().Error("whisperkit只支持macos", zap.String("当前系统", runtime.GOOS))
			return fmt.Errorf("whisperkit只支持macos")
		}
		if Conf.Transcribe.Whisperkit.Model == "" {
			return errors.New("检测到开启了whisperkit，但未配置模型名，请在配置文件中设置 transcribe.whisperkit.model")
		}
	case "whispercpp":
		if runtime.GOOS != "windows" { // 当前先仅支持win
			log.GetLogger().Error("whispercpp only support windows", zap.String("current os", runtime.GOOS))
			return fmt.Errorf("whispercpp only support windows")
		}
		if Conf.Transcribe.Whispercpp.Model == "" {
			return errors.New("检测到开启了whisper.cpp，但未配置模型名，请在配置文件中设置 transcribe.whispercpp.model")
		}
	case "aliyun":
		if Conf.Transcribe.Aliyun.Speech.AccessKeyId == "" || Conf.Transcribe.Aliyun.Speech.AccessKeySecret == "" || Conf.Transcribe.Aliyun.Speech.AppKey == "" {
			return errors.New("使用阿里云语音服务需要配置相关密钥")
		}
	default:
		return errors.New("不支持的转录提供商")
	}

	return nil
}

func LoadConfig() bool {
	var err error
	configPath := "./config/config.toml"
	if _, err = os.Stat(configPath); os.IsNotExist(err) {
		log.GetLogger().Info("未找到配置文件")
		return false
	} else {
		log.GetLogger().Info("已找到配置文件，从配置文件中加载配置")
		if _, err = toml.DecodeFile(configPath, &Conf); err != nil {
			log.GetLogger().Error("加载配置文件失败", zap.Error(err))
			return false
		}
		return true
	}
}

// 验证配置
func CheckConfig() error {
	var err error
	// 解析代理地址
	Conf.App.ParsedProxy, err = url.Parse(Conf.App.Proxy)
	if err != nil {
		return err
	}
	return validateConfig()
}

// SaveConfig 保存配置到文件
func SaveConfig() error {
	configPath := filepath.Join("config", "config.toml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(configPath), os.ModePerm)
		if err != nil {
			return err
		}
	}

	data, err := toml.Marshal(Conf)
	if err != nil {
		return err
	}

	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		return err
	}

	return nil
}
