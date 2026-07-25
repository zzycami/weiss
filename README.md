# weiss
一个go lib，允许通过本地proxy的方式直连pixiv  
# 实现
Android端实现:[Pixez](https://github.com/Notsfsssf/pixez-flutter)  
IOS端实现:存在信任证书问题  
Windows&mac:已验证可行

# compile
如果是go可以直接引用，gomod已经写好  
需要使用`goproxy`提供的证书生成方式生成自己的证书
```
cd ./goproxy/certs/
bash openssl-gen.sh
```
使用
```
weiss.Start("7890")
weiss.Stop()
``` 
# JSON 参数（`Start(port, json)` / weissd 第三参数）

除 host→IP 映射外，可设置 **`upstream_mode`**（与 iOS 设置「登录代理（Weiss）传输」一致）：

| 值 | 行为 |
|----|------|
| `auto`（默认） | 先走 HK `ech/config` 的 ECH；ECH TLS 握手失败再回退域名前置（硬编码/OneZero/DNS） |
| `ech_only` | 仅 ECH，不回退域名前置 |
| `fronting_only` | 不使用 ECH，仅域名前置 |

示例：`{"accounts.pixiv.net":"210.140.x.x","upstream_mode":"fronting_only"}`

# 缺陷  
`Doh`方式获取真实ip仍然存在cloudflare套壳的问题，需要及时硬编码更新  
可以修改`onezero.go`的`hardcodeIpMap`达成硬编码的目的  
黑名单虽然可以加速超时，但是会影响人机验证 

如果有什么更好的方法，欢迎交流
