package playbook

// FieldSpec 描述一个 action 字段
type FieldSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"` // text | textarea | int | bool | enum | list
	Enum        []string `json:"enum,omitempty"`
	Required    bool     `json:"required"`
	Help        string   `json:"help,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
}

// TagSpec 描述一种 action 类型
type TagSpec struct {
	Tag    string      `json:"tag"`
	Title  string      `json:"title"`
	Group  string      `json:"group"`
	Flow   bool        `json:"flow"` // 新增时默认用流式写法
	Fields []FieldSpec `json:"fields"`
}

var commonFields = []FieldSpec{
	{Key: "option", Label: "条件 (option)", Kind: "text", Help: "以 ! 开头表示取反；需在 playbook.conf 的 FeaturePages 中定义"},
	{Key: "options", Label: "条件组 (options)", Kind: "list"},
	{Key: "weight", Label: "权重 (weight)", Kind: "int", Help: "越大越晚执行"},
	{Key: "errorAction", Label: "错误处理", Kind: "enum", Enum: []string{"Stop", "Continue", "Ignore"}},
}

var tagSpecs = []TagSpec{
	{
		Tag: "registryValue", Title: "注册表值", Group: "注册表", Flow: true,
		Fields: []FieldSpec{
			{Key: "path", Label: "注册表路径", Kind: "text", Required: true, Placeholder: `HKCU\Software\...`},
			{Key: "value", Label: "值名称", Kind: "text"},
			{Key: "type", Label: "类型", Kind: "enum", Enum: []string{"REG_DWORD", "REG_SZ", "REG_EXPAND_SZ", "REG_QWORD", "REG_BINARY", "REG_MULTI_SZ"}},
			{Key: "data", Label: "数据", Kind: "text"},
			{Key: "operation", Label: "操作", Kind: "enum", Enum: []string{"add", "delete"}},
			{Key: "oobe", Label: "OOBE 限定", Kind: "enum", Enum: []string{"only"}},
		},
	},
	{
		Tag: "registryKey", Title: "注册表键", Group: "注册表", Flow: true,
		Fields: []FieldSpec{
			{Key: "path", Label: "注册表路径", Kind: "text", Required: true},
			{Key: "operation", Label: "操作", Kind: "enum", Enum: []string{"add", "delete"}},
		},
	},
	{
		Tag: "task", Title: "引用子任务", Group: "流程", Flow: true,
		Fields: []FieldSpec{
			{Key: "path", Label: "任务文件路径", Kind: "text", Required: true, Placeholder: `Tasks\registry\...yml`},
		},
	},
	{
		Tag: "writeStatus", Title: "写进度提示", Group: "流程", Flow: true,
		Fields: []FieldSpec{
			{Key: "status", Label: "提示文本", Kind: "text", Required: true},
		},
	},
	{
		Tag: "status", Title: "状态提示", Group: "流程", Flow: true,
		Fields: []FieldSpec{
			{Key: "status", Label: "提示文本", Kind: "text", Required: true},
		},
	},
	{
		Tag: "taskKill", Title: "结束进程", Group: "流程", Flow: true,
		Fields: []FieldSpec{
			{Key: "name", Label: "进程名", Kind: "text", Required: true},
		},
	},
	{
		Tag: "scheduledTask", Title: "计划任务", Group: "流程", Flow: true,
		Fields: []FieldSpec{
			{Key: "path", Label: "任务路径", Kind: "text", Required: true, Placeholder: `\\Microsoft\\Windows\\...`},
			{Key: "operation", Label: "操作", Kind: "enum", Enum: []string{"delete", "enable", "disable", "deleteFolder"}},
			{Key: "data", Label: "原始任务 XML (data)", Kind: "textarea", Help: "operation 为 enable 时用于注册新任务"},
		},
	},
	{
		Tag: "run", Title: "运行程序", Group: "执行", Fields: []FieldSpec{
			{Key: "exe", Label: "可执行文件", Kind: "text", Required: true},
			{Key: "args", Label: "参数", Kind: "text"},
			{Key: "path", Label: "工作目录", Kind: "text", Placeholder: `%ProgramFiles%\...`},
			{Key: "exeDir", Label: "在任务目录执行", Kind: "bool"},
			{Key: "wait", Label: "等待结束", Kind: "bool"},
			{Key: "showOutput", Label: "显示输出", Kind: "bool"},
			{Key: "showError", Label: "显示错误", Kind: "bool"},
			{Key: "runas", Label: "运行身份", Kind: "enum", Enum: []string{"currentUserElevated", "currentUser", "system", "trustedInstaller"}},
			{Key: "cpuArch", Label: "CPU 架构", Kind: "enum", Enum: []string{"X64", "Arm64", "Any"}},
			{Key: "package", Label: "所属软件包", Kind: "text"},
		},
	},
	{
		Tag: "cmd", Title: "CMD 命令", Group: "执行", Fields: []FieldSpec{
			{Key: "command", Label: "命令", Kind: "textarea", Required: true},
			{Key: "exeDir", Label: "在任务目录执行", Kind: "bool"},
			{Key: "ignoreErrors", Label: "忽略错误", Kind: "bool"},
			{Key: "runas", Label: "运行身份", Kind: "enum", Enum: []string{"currentUserElevated", "currentUser", "system", "trustedInstaller"}},
		},
	},
	{
		Tag: "powerShell", Title: "PowerShell", Group: "执行", Fields: []FieldSpec{
			{Key: "command", Label: "脚本", Kind: "textarea", Required: true},
			{Key: "exeDir", Label: "在任务目录执行", Kind: "bool"},
			{Key: "wait", Label: "等待结束", Kind: "bool"},
			{Key: "runas", Label: "运行身份", Kind: "enum", Enum: []string{"currentUserElevated", "currentUser", "system", "trustedInstaller"}},
		},
	},
	{
		Tag: "download", Title: "下载文件", Group: "执行", Fields: []FieldSpec{
			{Key: "url", Label: "下载地址", Kind: "text", Required: true},
			{Key: "destination", Label: "保存文件名", Kind: "text", Required: true},
			{Key: "overwrite", Label: "覆盖已存在", Kind: "bool"},
			{Key: "cpuArch", Label: "CPU 架构", Kind: "enum", Enum: []string{"X64", "Arm64", "Any"}},
			{Key: "package", Label: "所属软件包", Kind: "text"},
		},
	},
	{
		Tag: "service", Title: "服务配置", Group: "系统", Flow: true,
		Fields: []FieldSpec{
			{Key: "name", Label: "服务名", Kind: "text", Required: true},
			{Key: "operation", Label: "操作", Kind: "enum", Enum: []string{"change", "start", "stop", "delete"}},
			{Key: "startup", Label: "启动类型", Kind: "enum", Enum: []string{"0", "1", "2", "3", "4"}, Help: "0=引导 1=系统 2=自动 3=手动 4=禁用"},
		},
	},
	{
		Tag: "appx", Title: "Appx 操作", Group: "系统", Flow: true,
		Fields: []FieldSpec{
			{Key: "operation", Label: "操作", Kind: "enum", Enum: []string{"clearCache", "remove"}, Required: true},
			{Key: "name", Label: "包名/通配", Kind: "text", Required: true, Placeholder: "*Client.CBS*"},
		},
	},
	{
		Tag: "file", Title: "删除文件/目录", Group: "系统", Flow: true,
		Fields: []FieldSpec{
			{Key: "path", Label: "路径", Kind: "text", Required: true},
		},
	},
	{
		Tag: "software", Title: "安装软件包", Group: "系统", Fields: []FieldSpec{
			{Key: "name", Label: "名称", Kind: "text", Required: true},
			{Key: "source", Label: "来源", Kind: "enum", Enum: []string{"chocolatey", "winget"}},
			{Key: "package", Label: "包标识", Kind: "text"},
		},
	},
	{
		Tag: "systemPackage", Title: "系统包移除(已废弃)", Group: "系统",
		Fields: []FieldSpec{
			{Key: "name", Label: "包名", Kind: "text", Required: true},
		},
	},
}

// TagSpecs 返回全部 action 类型定义
func TagSpecs() []TagSpec { return tagSpecs }

// CommonFields 返回所有 action 通用的字段
func CommonFields() []FieldSpec { return commonFields }

// TagSpecOf 查找类型定义
func TagSpecOf(tag string) (TagSpec, bool) {
	for _, s := range tagSpecs {
		if s.Tag == tag {
			return s, true
		}
	}
	return TagSpec{}, false
}

// FieldKindOf 返回某 action 某字段的渲染类型，用于写回时决定是否加引号
func FieldKindOf(tag, key string) string {
	switch key {
	case "weight", "startup":
		return "int"
	case "exeDir", "wait", "ignoreErrors", "overwrite", "showOutput", "showError":
		return "bool"
	case "options":
		return "list"
	}
	if s, ok := TagSpecOf(tag); ok {
		for _, f := range s.Fields {
			if f.Key == key {
				switch f.Kind {
				case "int":
					return "int"
				case "bool":
					return "bool"
				case "list":
					return "list"
				}
			}
		}
	}
	for _, f := range commonFields {
		if f.Key == key {
			switch f.Kind {
			case "int":
				return "int"
			case "bool":
				return "bool"
			case "list":
				return "list"
			}
		}
	}
	return "auto"
}

// GroupNames 返回分组顺序
func GroupNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range tagSpecs {
		if !seen[s.Group] {
			seen[s.Group] = true
			out = append(out, s.Group)
		}
	}
	return out
}
