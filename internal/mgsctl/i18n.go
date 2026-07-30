package mgsctl

import "strings"

var tuiMessages = map[string]map[string]string{
	LanguageChinese: {
		"root.install":              "安装与部署",
		"root.runtime":              "运行维护",
		"root.setup":                "初始化设置",
		"root.upgrade":              "升级与配置迁移",
		"root.cluster":              "集群管理",
		"root.tool":                 "MGSCTL 工具",
		"root.language.zh":          "语言：中文",
		"root.language.en":          "语言：英文",
		"root.exit":                 "退出",
		"summary.install":           "安装新部署",
		"summary.import":            "导入旧版运行配置",
		"summary.status":            "查看部署状态",
		"summary.doctor":            "诊断部署健康状态",
		"summary.restart":           "重启部署服务",
		"summary.version":           "查看 mgsctl 构建信息",
		"summary.self_update":       "更新 mgsctl 可执行文件",
		"summary.upgrade":           "升级已部署应用",
		"summary.uninstall":         "停止服务或移除部署",
		"summary.setup_status":      "查看初始化状态",
		"summary.setup_show":        "显示当前初始化令牌",
		"summary.setup_reset":       "重置初始化令牌",
		"summary.cluster_token":     "创建一次性集群加入令牌",
		"summary.cluster_join":      "将节点加入集群",
		"field.mode":                "部署模式",
		"field.profile":             "部署方案",
		"field.topology":            "部署拓扑",
		"field.role":                "节点角色",
		"field.components":          "组件",
		"field.runtime-dir":         "运行目录",
		"field.storage-driver":      "对象存储",
		"field.public-api-url":      "公共 API 地址",
		"field.application-version": "应用版本",
		"field.image-registry":      "镜像仓库",
		"field.image-tag":           "镜像标签",
		"field.release-version":     "发布版本",
		"field.api-port":            "API 端口",
		"field.gateway-port":        "网关端口",
		"field.user-web-port":       "用户端端口",
		"field.admin-web-port":      "管理端端口",
		"field.docs-web-port":       "文档站端口",
		"field.monitoring-port":     "监控端口",
		"field.external-gateway":    "已配置外部网关",
		"field.migrate":             "执行数据库迁移",
		"field.overwrite":           "覆盖未完成配置",
		"field.yes":                 "跳过终端确认",
		"field.source":              "旧版环境文件",
		"field.json":                "JSON 输出",
		"field.version":             "目标版本",
		"field.release-base-url":    "Release 基础地址",
		"field.download-url":        "产物地址",
		"field.sha256":              "预期 SHA-256",
		"field.delete-data":         "删除持久化数据",
		"field.confirm":             "安装实例确认短语",
		"field.server":              "控制节点 API 地址",
		"field.token":               "一次性令牌",
		"field.ttl":                 "令牌有效期",
		"multi.status":              "已选择 %d 项；左右键浏览",
		"multi.readonly":            "方案预设，只读",
		"validation.prefix":         "校验错误",
		"warning.prefix":            "提示",
		"review.title":              "确认命令",
		"nav.menu":                  "方向键导航 · Enter 确认 · Space 切换 · Esc 返回 · Ctrl+C 退出",
		"nav.form":                  "方向键导航 · Tab 切换字段 · Space 选择 · Enter 预览 · Esc 返回 · Ctrl+C 退出",
		"nav.review":                "Enter 执行 · Esc 修改 · Ctrl+C 退出",
	},
	LanguageEnglish: {
		"root.install":              "Install and deployment",
		"root.runtime":              "Runtime operations",
		"root.setup":                "Setup initialization",
		"root.upgrade":              "Upgrade and configuration migration",
		"root.cluster":              "Cluster management",
		"root.tool":                 "MGSCTL tool",
		"root.language.zh":          "Language: Chinese",
		"root.language.en":          "Language: English",
		"root.exit":                 "Exit",
		"summary.install":           "Install a new deployment",
		"summary.import":            "Import a legacy runtime configuration",
		"summary.status":            "Show deployment status",
		"summary.doctor":            "Diagnose deployment health",
		"summary.restart":           "Restart deployment services",
		"summary.version":           "Show mgsctl build information",
		"summary.self_update":       "Update the mgsctl executable",
		"summary.upgrade":           "Upgrade the deployed application",
		"summary.uninstall":         "Stop services or remove a deployment",
		"summary.setup_status":      "Show Setup initialization status",
		"summary.setup_show":        "Show the current Setup token",
		"summary.setup_reset":       "Reset the Setup token",
		"summary.cluster_token":     "Create a single-use cluster join token",
		"summary.cluster_join":      "Join a node to a cluster",
		"field.mode":                "Mode",
		"field.profile":             "Profile",
		"field.topology":            "Topology",
		"field.role":                "Role",
		"field.components":          "Components",
		"field.runtime-dir":         "Runtime directory",
		"field.storage-driver":      "Object storage",
		"field.public-api-url":      "Public API URL",
		"field.application-version": "Application version",
		"field.image-registry":      "Image registry",
		"field.image-tag":           "Image tag",
		"field.release-version":     "Release version",
		"field.api-port":            "API port",
		"field.gateway-port":        "Gateway port",
		"field.user-web-port":       "User Web port",
		"field.admin-web-port":      "Admin Web port",
		"field.docs-web-port":       "Docs Web port",
		"field.monitoring-port":     "Monitoring port",
		"field.external-gateway":    "External gateway configured",
		"field.migrate":             "Run migration",
		"field.overwrite":           "Overwrite incomplete config",
		"field.yes":                 "Skip terminal confirmation",
		"field.source":              "Legacy environment file",
		"field.json":                "JSON output",
		"field.version":             "Target version",
		"field.release-base-url":    "Release base URL",
		"field.download-url":        "Artifact URL",
		"field.sha256":              "Expected SHA-256",
		"field.delete-data":         "Delete persistent data",
		"field.confirm":             "Installation-specific phrase",
		"field.server":              "Control API URL",
		"field.token":               "Single-use token",
		"field.ttl":                 "Token lifetime",
		"multi.status":              "%d selected; Left/Right browses",
		"multi.readonly":            "profile preset, read-only",
		"validation.prefix":         "Validation error",
		"warning.prefix":            "Notice",
		"review.title":              "Review command",
		"nav.menu":                  "Arrow keys navigate · Enter confirms · Space toggles · Esc returns · Ctrl+C exits",
		"nav.form":                  "Arrow keys navigate · Tab changes field · Space toggles · Enter reviews · Esc returns · Ctrl+C exits",
		"nav.review":                "Enter confirms · Esc edits · Ctrl+C exits",
	},
}

func tuiMessage(language, key string) string {
	if !supportedLanguage(language) {
		language = LanguageChinese
	}
	if value := tuiMessages[language][key]; value != "" {
		return value
	}
	if value := tuiMessages[LanguageChinese][key]; value != "" {
		return value
	}
	return key
}

func tuiCatalogSummary(language string, entry CommandCatalogEntry) string {
	key := map[CommandKind]string{
		CommandInstall: "summary.install", CommandImportConfig: "summary.import", CommandStatus: "summary.status",
		CommandDoctor: "summary.doctor", CommandRestart: "summary.restart", CommandVersion: "summary.version",
		CommandSelfUpdate: "summary.self_update", CommandUpgrade: "summary.upgrade", CommandUninstall: "summary.uninstall",
		CommandSetupStatus: "summary.setup_status", CommandSetupTokenShow: "summary.setup_show", CommandSetupTokenReset: "summary.setup_reset",
		CommandClusterTokenCreate: "summary.cluster_token", CommandClusterJoin: "summary.cluster_join",
	}[entry.Kind]
	if key == "" {
		return entry.Summary
	}
	return tuiMessage(language, key)
}

func normalizedTUILanguage(language string) string {
	if supportedLanguage(strings.TrimSpace(language)) {
		return strings.TrimSpace(language)
	}
	return LanguageChinese
}
