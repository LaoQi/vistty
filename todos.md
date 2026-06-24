# xterm-256 VT 支持改进计划

## 决策记录
- terminal 层测试：提取可测试核心（`hostWriter` 字段 + `feedBytes` + `newTerminalForTest`）
- 实现范围：P0 + P1，去除鼠标支持（?1000/?1002/?1006 不做）
- DA1 响应身份：`ESC[?62;4c`（VT220 + SGR 颜色）
- 字符集：DEC line drawing（`ESC ( 0`）+ G1/SO/SI 全部纳入
- `?1004` 焦点：仅存标志位，预留平台接入
- `?2004` 括号粘贴：仅存标志位 + 预留粘贴包裹

## 缺陷清单（全部已修复）

### 解析层 (`internal/vte/`)
- [x] `csi.go` `CSI q` 不检查 intermediate → DECSCUSR/DECSCA/裸q 混淆 → 按 intermed 分发
- [x] `sgr.go` SGR 22 只发 BoldOff，漏 DimOff → 同时返回 BoldOff + DimOff
- [x] 缺 CSI 命令：X(ECH)、n(DSR)、c(DA1)、>c(DA2)、g(TBC) → 全部新增
- [x] `csi.go` 私有标记 `>`/`=`/`<` 不分发 → parseCSIPrivate 按 marker 分发
- [x] `esc.go` 不识别 `ESC ( B/0`（G0/G1 字符集）→ 新增 ESCDesignateG0/G1
- [x] `osc.go` 缺 OSC 2（窗口标题）→ 新增

### 执行层 (`terminal/terminal.go`)
- [x] `handleMode` 仅 ?25/?1049 → 扩展 ?1/?7/?47/?1047/?1048/?1004/?2004
- [x] DECSC/DECRC 只存行列 → savedCursorState 含 SGR/charset
- [x] `eraseDisplay` 缺 case 3 → 新增清 scrollback
- [x] 无响应回写通道 → hostWriter 字段 + ptyWrite 改写
- [x] `execCSI` 缺 ECH/DSR/DA1/DA2/TBC → 全部新增
- [x] `setTitle` 空实现 → 调用 opts.OnTitle
- [x] `writeKeyEscape` 硬编码 → 应用光标键模式
- [x] 无字符集状态 → charset.go + execPrint 转换 + SO/SI

## 实施阶段

### 阶段 0：测试基础设施 [x]
- [x] `terminal.go` 新增 `hostWriter io.Writer` 字段，`ptyWrite` 改写它，`New()` 置 ptyFile
- [x] `terminal.go` 新增 `feedBytes([]byte)` 方法
- [x] `terminal_test.go` 新增 `newTerminalForTest(cols,rows)` 辅助
- [x] 验证：现有测试全部通过

### 阶段 1：vte 解析层 [x]
- [x] 1.1 `sgr.go` SGR 22 修复 → `sgr_test.go` 测试
- [x] 1.2 `csi.go` 新增枚举 ECH/DSR/DA1/DA2/TBC/DECSCA + q 按 intermed 分发 + 私有 > 分发 → `csi_test.go` 测试
- [x] 1.3 `esc.go` 新增 G0/G1 字符集枚举 + `( ) * +` 识别 → `esc_test.go` 测试
- [x] 1.4 `osc.go` 新增 OSC 2 → `osc_test.go` 测试
- [x] 1.5 `parser_test.go` 补 intermed/私有标记/连续 OSC 测试
- [x] 验证：86 测试全通过

### 阶段 2：terminal 执行层 [x]
- [x] 2.1 光标/擦除：ECH(X)、ED case3、TBC(g) → `terminal_csi_test.go`
- [x] 2.2 DSR/DA 响应回写 → `terminal_response_test.go`
- [x] 2.3 模式标志位：DECAWM(?7)、DECCKM(?1)、?2004、?47/1047/1048、?1004 → `terminal_mode_test.go`
- [x] 2.4 DECSC/DECRC 属性保存（savedState 重构）→ `terminal_state_test.go`
- [x] 2.5 DECSCUSR 闪烁/稳定 → `terminal_cursor_test.go`
- [x] 2.6 字符集：charset.go + execPrint 转换 + SO/SI → `terminal_charset_test.go`
- [x] 2.7 OSC 标题落地（OnTitle）→ `terminal_osc_test.go`
- [x] 验证：`go vet ./... && go test ./...` 全通过

## 审计记录

### 阶段 0 审计
- 状态：通过
- 改动：terminal.go 新增 hostWriter 字段 + feedBytes 方法；terminal_test.go 新增 newTerminalForTest
- 测试：现有 3 测试全通过（2 SKIP 因缺字体）
- 风险：零侵入生产代码，ptyWrite 从 t.pty.Write 改为 t.hostWriter.Write，New() 中 hostWriter=ptyFile 保持原语义

### 阶段 1 审计
- 状态：通过
- 改动：
  - sgr.go: case 22 返回 [BoldOff, DimOff]；parseSGRColor 无效子模式 advance 改 2
  - csi.go: 新增 6 枚举；privateMarker 检测改为 ?/>/=/<；q 按 intermed 区分 DECSCUSR/DECSCA；新增 X/n/c/g；parseCSIPrivate 支持 ?n(DSR) 和 >c(DA2)
  - esc.go: ESCSequence.Intermed 类型 byte→[]byte，新增 Charset 字段；识别 ( ) * + 指定 G0/G1
  - osc.go: 新增 OSC 2
- 测试：86 PASS / 0 FAIL（原有 65 + 新增 21）
- 注意：TestSGRResetAttributes 原有断言因 SGR22 行为变更而更新（22 现返回 2 个 SGR）

### 阶段 2 审计
- 状态：通过
- 改动：
  - charset.go（新）：DEC line drawing 映射表 + charsetState（G0/G1/GL）+ Translate
  - terminal.go: savedCursorState 结构体（含 SGR/charset）；新增字段 autoWrap/cursorKeysApp/bracketedPaste/focusReporting/charset/tabStops；saveCursor/restoreCursor 完整状态；tab stop 管理（init/set/clear/next/prev）；execCSI 新增 ECH/DSR/DA1/DA2/TBC；DECSCUSR 6 种样式含闪烁；handleMode 扩展 ?1/?7/?47/?1047/?1048/?1004/?2004；execESC 新增 G0/G1 + ESCTabSet；execPrint 字符集转换 + autoWrap；execControl SO/SI + HT tabStops；eraseDisplay case 3；eraseChars；handleDSR；writeKeyEscape 应用光标键；setTitle 落地
  - options.go: 新增 OnTitle 字段
- 测试：terminal 包 33 PASS / 0 FAIL（7 个新测试文件）
- 总计：vte 86 PASS + terminal 33 PASS = 119 PASS / 0 FAIL / 3 SKIP

## 文件改动总览

| 文件 | 类型 | 行数变化 |
|------|------|---------|
| `internal/vte/csi.go` | 改 | 154→197 (+43) |
| `internal/vte/sgr.go` | 改 | 177→177 (case 22 修改) |
| `internal/vte/esc.go` | 改 | 56→63 (+7, Intermed 类型变更 + G0/G1) |
| `internal/vte/osc.go` | 改 | 66→68 (+2, OSC 2) |
| `internal/vte/csi_test.go` | 改 | 107→202 (+95) |
| `internal/vte/sgr_test.go` | 改 | 155→219 (+64) |
| `internal/vte/esc_test.go` | 改 | 85→133 (+48) |
| `internal/vte/osc_test.go` | 改 | 80→91 (+11) |
| `internal/vte/parser_test.go` | 改 | 255→307 (+52) |
| `terminal/charset.go` | 新 | 97 行 |
| `terminal/terminal.go` | 改 | 1078→1200+ (+~122) |
| `terminal/options.go` | 改 | 25→27 (+2) |
| `terminal/terminal_test.go` | 改 | 165→190 (+25, newTerminalForTest) |
| `terminal/terminal_csi_test.go` | 新 | ~60 行 |
| `terminal/terminal_response_test.go` | 新 | ~55 行 |
| `terminal/terminal_mode_test.go` | 新 | ~133 行 |
| `terminal/terminal_state_test.go` | 新 | ~50 行 |
| `terminal/terminal_cursor_test.go` | 新 | ~45 行 |
| `terminal/terminal_charset_test.go` | 新 | ~52 行 |
| `terminal/terminal_osc_test.go` | 新 | ~30 行 |
