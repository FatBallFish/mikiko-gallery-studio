# 拆分 mgsctl 与应用镜像发布流水线技术方案

> 日期：2026-09-11
> 状态：已确认
> 需求来源：`docs/prd/2026-09-11-split-mgsctl-app-release-requirements.md`

## 1. 现状与耦合点

`.github/workflows/release.yml` 由 `v*` tag 触发，单次发布同时产出：

- mgsctl 六平台二进制（`build-mgsctl` job）
- 原生包、后端包、前端包（三个打包 job）
- 五个多架构 Docker 镜像（`build-images` job）
- `release-manifest.json`（绑定镜像 digest 与全部资产校验和）
- GitHub Release 资产上传与 `latest` 镜像提升

消费方对发布布局的依赖：

| 消费方 | 路径 | 依赖 |
| --- | --- | --- |
| `scripts/install.sh` | `releases/latest/download/mgsctl-<os>-<arch>` | GitHub Latest Release 必须携带 mgsctl 资产 |
| `mgsctl self-update`（新与旧） | 同上，或 `releases/download/<version>/mgsctl-*` | 同上 |
| `mgsctl install/upgrade`（应用 latest） | `releases/latest/download/release-manifest.json` | GitHub Latest Release 必须携带 manifest |

拆分后应用 Release 不再包含 mgsctl 资产、mgsctl Release 不包含 manifest，GitHub 的单一 Latest Release 标记无法同时满足两条 "latest" 语义。

## 2. 设计

### 2.1 Tag 与触发

- 应用发布：保留 `v*` tag 触发 `release.yml`（改名 job 语义，删除 `build-mgsctl`）。
- mgsctl 发布：新增 `mgsctl-v*` tag 触发 `release-mgsctl.yml`，仅做 mgsctl 相关验证（`go vet`、`go test ./internal/mgsctl`、install wrapper 合同）与六平台打包。

### 2.2 Latest 归属

- mgsctl Release 创建时使用 `gh release create --latest`，使 `releases/latest/download/mgsctl-*` 永远解析到最新 mgsctl；这保持 `install.sh` 与**线上已部署旧版 mgsctl** 的 self-update 路径不变。
- 应用 Release 创建时使用 `gh release create --latest=false`，避免抢占 Latest 标记。

### 2.3 应用 latest manifest 解析

`internal/mgsctl/release_manifest.go` 的 `ResolveReleaseManifest` 在 selector 为 `latest` 时：

1. 调用 GitHub API `GET https://api.github.com/repos/<owner>/<repo>/releases?per_page=100`（由 `ReleaseBaseURL` 推导，仅接受 github.com Release 基址），按返回顺序（新到旧）找到第一个 `assets` 含 `release-manifest.json` 的非 draft Release，取该资产的 `browser_download_url` 下载 manifest 与校验和。
2. API 不可用、列表为空或资产缺失时，回退现行 `latest/download/release-manifest.json` 路径。这兼容历史统一发布与非 GitHub 部署。
3. 校验与现有逻辑一致（sha256 校验、schema、镜像 digest）。

私有仓库场景沿用现状：当前实现同样依赖匿名 `github.com` 下载，不引入 token；后续需要时可扩展 `GITHUB_TOKEN` 头。

### 2.4 版本语义

- mgsctl 二进制版本 = `mgsctl-vX.Y.Z`（`package-mgsctl.sh` 的 `RELEASE_VERSION` 直接使用完整 tag，buildinfo 不要求 `v` 前缀）。
- `mgsctl self-update --version mgsctl-vX.Y.Z` 走 `releases/download/<tag>/mgsctl-*`，无需改动。
- 应用 manifest 的 `application_version` 与镜像 tag 仍为 `vX.Y.Z`，mgsctl 二进制版本与应用版本互不比较（现状即如此，仅展示）。

### 2.5 合同测试

`scripts/devops/release-contract-test.sh` 改为同时约束两个 workflow 文件：

- `release.yml`：保留镜像/安装包/manifest/release 相关断言；新增禁止出现 `package-mgsctl.sh` 与 `build-mgsctl`；要求 `--latest=false`。
- `release-mgsctl.yml`：要求 `mgsctl-v*` 触发、六平台矩阵、`package-mgsctl.sh`、`gh release create`、`--latest`、资产与 `.sha256` 上传、mgsctl 专属验证步骤。
- manifest fixture 不再依赖 mgsctl 资产存在（应用 Release 已无 mgsctl 资产，renderer 的"至少一个校验和资产"由安装包满足）。

## 3. 兼容性

- 线上已部署 mgsctl（v0.0.27）：`self-update`（latest 或显式旧版本）不受影响；`upgrade --image-tag latest` 因 Latest Release 改由 mgsctl 发布占据，旧二进制内的 `latest/download/release-manifest.json` 会 404——需要先 `self-update` 再升级应用。此窗口期说明写入 runbook。
- 历史统一 Release 资产保持原样，显式版本解析不受影响。

## 4. 测试

- Go：`ResolveReleaseManifest` latest 分支新增单测——API 命中带 manifest 的 Release、API 无命中回退 legacy 路径、API 失败回退 legacy 路径、非 GitHub 基址回退。
- 合同测试覆盖两个 workflow 的关键文本。
- `verify.sh`、隔离 API smoke、committed review gate 全部通过。
