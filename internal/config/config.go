package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Backend  string  `json:"backend"`
	Shell    string  `json:"shell"`
	Font     string  `json:"font"`
	FontSize float64 `json:"fontsize"`
	Primary  string  `json:"primary"`
	Record   string  `json:"record"`
	ErrorLog string  `json:"error_log"`
}

func Default() Config {
	return Config{
		Backend:  "auto",
		Shell:    "/bin/bash",
		Font:     "",
		FontSize: 14,
		Primary:  "",
		Record:   "",
		ErrorLog: "",
	}
}

func DefaultPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "vistty", "config.jsonc"), nil
}

func (c Config) Generate() string {
	return fmt.Sprintf(`{
  // 显示后端: "auto" | "wayland" | "drm" | "drm-gbm"
  "backend": %q,
  // 启动的 shell 程序路径
  "shell": %q,
  // 字体文件路径 (空字符串表示使用内置字体)
  "font": %q,
  // 字体大小 (像素)
  "fontsize": %g,
  // 主输出名称或索引 (如 "HDMI-A-1" 或 "0")
  "primary": %q,
  // 录制 PTY 输出到指定文件
  "record": %q,
  // 错误日志文件路径 (默认 ~/.local/share/vistty/error.log)
  "error_log": %q
}
`, c.Backend, c.Shell, c.Font, c.FontSize, c.Primary, c.Record, c.ErrorLog)
}

func stripComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false
	i := 0
	for i < len(data) {
		c := data[i]
		if inString {
			out = append(out, c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			i++
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			i++
			continue
		}
		if c == '/' && i+1 < len(data) {
			if data[i+1] == '/' {
				for i < len(data) && data[i] != '\n' {
					i++
				}
				continue
			}
			if data[i+1] == '*' {
				i += 2
				for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
					i++
				}
				i += 2
				if i > len(data) {
					i = len(data)
				}
				continue
			}
		}
		out = append(out, c)
		i++
	}
	return out
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(stripComments(data), &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(c.Generate()), 0644)
}
