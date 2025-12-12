# Weiss.xcframework 构建说明

## 架构支持

新构建的 `Weiss.xcframework` 支持以下平台和架构：

- **iOS 设备**: arm64
- **iOS 模拟器**:
  - arm64 (Apple Silicon Mac)
  - x86_64 (Intel Mac)

## 构建步骤

### 1. 安装依赖

```bash
# 安装 gomobile
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest

# 初始化 gomobile
~/go/bin/gomobile init
```

### 2. 构建 xcframework

```bash
# 在项目根目录执行
./build_ios.sh
```

构建脚本会自动：
- 清理旧的构建产物
- 构建 iOS 设备版本（arm64）
- 构建 iOS 模拟器版本（arm64 + x86_64）
- 修正模块名为 "Weiss" 以保持代码兼容性
- 创建统一的 xcframework
- 验证架构支持

### 3. 更新到项目中

```bash
# 将构建好的 xcframework 复制到 pixiv-client 项目
cp -r Weiss.xcframework /path/to/pixiv-client/
```

## 验证架构

构建完成后，可以使用以下命令验证架构：

```bash
# 验证 iOS 设备版本
lipo -info Weiss.xcframework/ios-arm64/Weiss-Ios.framework/Weiss-Ios

# 验证 iOS 模拟器版本
lipo -info Weiss.xcframework/ios-arm64_x86_64-simulator/Weiss-Iossimulator.framework/Weiss-Iossimulator
```

## 注意事项

1. **模块名称**: 构建脚本会自动将模块名从 "Weiss-Ios"/"Weiss-Iossimulator" 改为 "Weiss"，以保持与现有代码的兼容性
2. **Framework 名称**: 虽然 framework 文件名是 `Weiss-Ios.framework` 和 `Weiss-Iossimulator.framework`，但模块名仍然是 "Weiss"，所以代码中使用 `import Weiss` 即可
3. **Apple Silicon 支持**: 新构建的版本完全支持 Apple Silicon Mac 上的模拟器

## 构建环境

- **操作系统**: macOS (支持 Apple Silicon 和 Intel)
- **Xcode**: 需要安装最新版本的 Xcode 和命令行工具
- **Go**: 需要安装 Go 1.19 或更高版本
