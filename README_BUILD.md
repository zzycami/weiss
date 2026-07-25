# Weiss.xcframework 构建说明

## 架构支持

`Weiss.xcframework` 支持以下平台（同一 XCFramework，Xcode 按 SDK 自动选切片）：

- **iOS 设备**: arm64
- **iOS 模拟器**: arm64 (Apple Silicon) + x86_64 (Intel)
- **Mac Catalyst**: arm64 (+ x86_64，视 gomobile 产出而定)

## 构建步骤

### 1. 安装依赖

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
~/go/bin/gomobile init
```

### 2. 构建 xcframework

```bash
./build_ios.sh
```

脚本会：

- 清理旧产物
- `gomobile bind`：`ios` / `iossimulator` / `maccatalyst`
- 统一模块名为 `Weiss`、框架名为 `Weiss.framework`
- `xcodebuild -create-xcframework` 合并三片
- 校验各切片二进制架构

### 3. 更新到 pixiv-client

```bash
rm -rf /path/to/pixiv-client/Weiss.xcframework
cp -R Weiss.xcframework /path/to/pixiv-client/
```

iOS VIP / Lite 与 `pixiv-client-mac`（Mac Catalyst）共用这一份 XCFramework，无需再维护单独的 `mac` git 分支。

## 验证

```bash
find Weiss.xcframework -name Weiss -type f -exec lipo -info {} \;
plutil -p Weiss.xcframework/Info.plist | head
```

## 注意事项

1. **模块名**: 统一为 `Weiss`，代码侧继续 `import Weiss`
2. **MinimumOSVersion**: iOS 切片设为 13.0；Mac Catalyst 切片设为 15.0（`gomobile -iosversion=15.0`；Xcode 会拒绝 `ios13.0-macabi`）
3. **Catalyst 版本化 framework**: gomobile 产出 `Weiss-Maccatalyst` 二进制；`build_ios.sh` 会重命名为 `Weiss` 并修复根符号链接，否则会出现 `ld: framework 'Weiss' not found`
4. **与 libcurl**: Mac Catalyst 登录链路依赖 vendored OpenSSL/curl（见 pixiv-client 的 STURLSession），勿与系统 libcurl 混用不同版本头文件
