package playbook

import (
	"fmt"
	"strings"
)

// playbook.conf 是 XML，这里用“扫描 + 定点替换”的方式编辑，
// 与 YAML 一致：只改动目标位置，其余内容（缩进、注释、属性顺序）原样保留。

type xmlAttr struct {
	Name       string
	Start, End int // 值内容（不含引号）
}

type xmlElem struct {
	Tag         string
	Start, End  int
	InnerStart  int
	InnerEnd    int
	Attrs       []xmlAttr
	SelfClosing bool
	Depth       int
	Parent      int
}

type xmlDoc struct {
	Text  string
	Elems []xmlElem
}

func isSpaceByte(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

func scanAttrs(body string, base int) []xmlAttr {
	var out []xmlAttr
	i := 0
	for i < len(body) {
		for i < len(body) && isSpaceByte(body[i]) {
			i++
		}
		s := i
		for i < len(body) && body[i] != '=' && !isSpaceByte(body[i]) {
			i++
		}
		if s == i {
			break
		}
		name := body[s:i]
		for i < len(body) && isSpaceByte(body[i]) {
			i++
		}
		if i >= len(body) || body[i] != '=' {
			continue
		}
		i++
		for i < len(body) && isSpaceByte(body[i]) {
			i++
		}
		if i >= len(body) {
			break
		}
		if q := body[i]; q == '"' || q == '\'' {
			i++
			vs := i
			for i < len(body) && body[i] != q {
				i++
			}
			out = append(out, xmlAttr{Name: name, Start: base + vs, End: base + i})
			i++
		} else {
			vs := i
			for i < len(body) && !isSpaceByte(body[i]) {
				i++
			}
			out = append(out, xmlAttr{Name: name, Start: base + vs, End: base + i})
		}
	}
	return out
}

func scanXML(text string) *xmlDoc {
	d := &xmlDoc{Text: text}
	var stack []int
	i := 0
	for i < len(text) {
		if text[i] != '<' {
			i++
			continue
		}
		rest := text[i:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			j := strings.Index(rest, "-->")
			if j < 0 {
				return d
			}
			i += j + 3
			continue
		case strings.HasPrefix(rest, "<?"):
			j := strings.Index(rest, "?>")
			if j < 0 {
				return d
			}
			i += j + 2
			continue
		case strings.HasPrefix(rest, "<![CDATA["):
			j := strings.Index(rest, "]]>")
			if j < 0 {
				return d
			}
			i += j + 3
			continue
		case strings.HasPrefix(rest, "<!"):
			j := strings.Index(rest, ">")
			if j < 0 {
				return d
			}
			i += j + 1
			continue
		case len(rest) > 1 && rest[1] == '/':
			j := strings.Index(rest, ">")
			if j < 0 {
				return d
			}
			if n := len(stack); n > 0 {
				idx := stack[n-1]
				d.Elems[idx].End = i + j + 1
				d.Elems[idx].InnerEnd = i
				stack = stack[:n-1]
			}
			i += j + 1
			continue
		}
		k := i + 1
		for k < len(text) && !isSpaceByte(text[k]) && text[k] != '>' && text[k] != '/' {
			k++
		}
		tag := text[i+1 : k]
		if tag == "" {
			i++
			continue
		}
		j := k
		var inQ byte
		for j < len(text) {
			c := text[j]
			switch {
			case inQ != 0:
				if c == inQ {
					inQ = 0
				}
			case c == '"' || c == '\'':
				inQ = c
			case c == '>':
				goto found
			}
			j++
		}
		return d
	found:
		e := xmlElem{Tag: tag, Start: i, End: j + 1, Parent: -1}
		if n := len(stack); n > 0 {
			e.Parent = stack[n-1]
			e.Depth = d.Elems[stack[n-1]].Depth + 1
		}
		body := text[k:j]
		e.Attrs = scanAttrs(body, k)
		if strings.HasSuffix(strings.TrimSpace(body), "/") {
			e.SelfClosing = true
			e.InnerStart, e.InnerEnd = j+1, j+1
		} else {
			e.InnerStart, e.InnerEnd = j+1, j+1
		}
		d.Elems = append(d.Elems, e)
		if !e.SelfClosing {
			stack = append(stack, len(d.Elems)-1)
		}
		i = j + 1
	}
	return d
}

// Conf 表示 playbook.conf
type Conf struct {
	Path string `json:"path"`
	Text string `json:"text"`
	CRLF bool   `json:"-"`

	doc *xmlDoc
}

// ParseConf 解析 playbook.conf
func ParseConf(path string, data []byte) (*Conf, error) {
	raw := string(data)
	crlf := strings.Contains(raw, "\r\n")
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	c := &Conf{Path: path, Text: text, CRLF: crlf}
	c.doc = scanXML(text)
	if len(c.doc.Elems) == 0 {
		return nil, fmt.Errorf("playbook.conf 解析失败：没有找到元素")
	}
	return c, nil
}

// Bytes 返回文本（保留原换行风格）
func (c *Conf) Bytes() []byte {
	if c.CRLF {
		return []byte(strings.ReplaceAll(c.Text, "\n", "\r\n"))
	}
	return []byte(c.Text)
}

// SetRaw 整体替换
func (c *Conf) SetRaw(s string) {
	c.Text = strings.ReplaceAll(s, "\r\n", "\n")
	c.doc = scanXML(c.Text)
}

// elemIndex 找第 occurrence 个同名元素
func (c *Conf) elemIndex(tag string, occurrence int) (int, error) {
	n := 0
	for i := range c.doc.Elems {
		if c.doc.Elems[i].Tag == tag {
			if n == occurrence {
				return i, nil
			}
			n++
		}
	}
	return -1, fmt.Errorf("未找到元素 <%s> #%d", tag, occurrence)
}

func (c *Conf) hasChildren(i int) bool {
	for j := range c.doc.Elems {
		if c.doc.Elems[j].Parent == i {
			return true
		}
	}
	return false
}

// ConfField 是可编辑的叶子元素
type ConfField struct {
	Tag        string `json:"tag"`
	Occurrence int    `json:"occurrence"`
	Path       string `json:"path"`
	Label      string `json:"label"`
	Value      string `json:"value"`
	Kind       string `json:"kind"` // text | bool
}

// ConfAttrItem 是可编辑的元素属性
type ConfAttrItem struct {
	Tag        string `json:"tag"`
	Occurrence int    `json:"occurrence"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	Value      string `json:"value"`
}

// occIndexes 计算每个元素在“同名元素”中的序号
func (c *Conf) occIndexes() []int {
	out := make([]int, len(c.doc.Elems))
	cnt := map[string]int{}
	for i := range c.doc.Elems {
		t := c.doc.Elems[i].Tag
		out[i] = cnt[t]
		cnt[t]++
	}
	return out
}

func (c *Conf) paths() []string {
	occ := c.occIndexes()
	paths := make([]string, len(c.doc.Elems))
	for i := range c.doc.Elems {
		e := c.doc.Elems[i]
		p := e.Tag
		if e.Parent >= 0 {
			p = paths[e.Parent] + "/" + e.Tag
		}
		if occ[i] > 0 {
			p = fmt.Sprintf("%s[%d]", p, occ[i])
		}
		paths[i] = p
	}
	return paths
}

func (c *Conf) text(i int) string {
	e := c.doc.Elems[i]
	if e.InnerEnd > e.InnerStart {
		return strings.TrimSpace(c.Text[e.InnerStart:e.InnerEnd])
	}
	return ""
}

func (c *Conf) attr(i int, name string) (string, bool) {
	for _, a := range c.doc.Elems[i].Attrs {
		if a.Name == name {
			return c.Text[a.Start:a.End], true
		}
	}
	return "", false
}

// Fields 返回全部叶子元素（可直接编辑）
func (c *Conf) Fields() []ConfField {
	occ := c.occIndexes()
	paths := c.paths()
	var out []ConfField
	for i := range c.doc.Elems {
		if c.hasChildren(i) {
			continue
		}
		v := c.text(i)
		kind := "text"
		lv := strings.ToLower(v)
		if lv == "true" || lv == "false" {
			kind = "bool"
		}
		out = append(out, ConfField{
			Tag: c.doc.Elems[i].Tag, Occurrence: occ[i], Path: paths[i],
			Label: c.doc.Elems[i].Tag, Value: v, Kind: kind,
		})
	}
	return out
}

// Attrs 返回全部元素属性
func (c *Conf) Attrs() []ConfAttrItem {
	occ := c.occIndexes()
	paths := c.paths()
	var out []ConfAttrItem
	for i := range c.doc.Elems {
		for _, a := range c.doc.Elems[i].Attrs {
			out = append(out, ConfAttrItem{
				Tag: c.doc.Elems[i].Tag, Occurrence: occ[i], Path: paths[i],
				Name: a.Name, Value: c.Text[a.Start:a.End],
			})
		}
	}
	return out
}

// ConfOptionItem 是 FeaturePages 中的一个选项
type ConfOptionItem struct {
	Tag            string `json:"tag"`
	Occurrence     int    `json:"occurrence"`
	Name           string `json:"name"`
	NameOccurrence int    `json:"nameOccurrence"`
	Text           string `json:"text"`
	TextOccurrence int    `json:"textOccurrence"`
	IsChecked      bool   `json:"isChecked"`
	HasChecked     bool   `json:"hasChecked"`
	IsNone         bool   `json:"isNone"`
}

// ConfOptionPage 是 FeaturePages 中的一页
type ConfOptionPage struct {
	Tag           string           `json:"tag"`
	Occurrence    int              `json:"occurrence"`
	Kind          string           `json:"kind"` // checkbox | radio | radioImage
	Description   string           `json:"description"`
	IsRequired    bool             `json:"isRequired"`
	DefaultOption string           `json:"defaultOption"`
	DependsOn     string           `json:"dependsOn"`
	Options       []ConfOptionItem `json:"options"`
}

func (c *Conf) descendants(idx int) []int {
	var out []int
	for i := range c.doc.Elems {
		p := c.doc.Elems[i].Parent
		for p >= 0 {
			if p == idx {
				out = append(out, i)
				break
			}
			p = c.doc.Elems[p].Parent
		}
	}
	return out
}

// OptionPages 解析 FeaturePages 结构
func (c *Conf) OptionPages() []ConfOptionPage {
	occ := c.occIndexes()
	var out []ConfOptionPage
	for i := range c.doc.Elems {
		e := c.doc.Elems[i]
		var kind string
		switch e.Tag {
		case "CheckboxPage":
			kind = "checkbox"
		case "RadioPage":
			kind = "radio"
		case "RadioImagePage":
			kind = "radioImage"
		default:
			continue
		}
		page := ConfOptionPage{Tag: e.Tag, Occurrence: occ[i], Kind: kind}
		if v, ok := c.attr(i, "Description"); ok {
			page.Description = v
		}
		if v, ok := c.attr(i, "IsRequired"); ok {
			page.IsRequired = strings.EqualFold(v, "true")
		}
		if v, ok := c.attr(i, "DefaultOption"); ok {
			page.DefaultOption = v
		}
		if v, ok := c.attr(i, "DependsOn"); ok {
			page.DependsOn = v
		}
		for _, d := range c.descendants(i) {
			dt := c.doc.Elems[d].Tag
			if dt != "CheckboxOption" && dt != "RadioOption" && dt != "RadioImageOption" {
				continue
			}
			item := ConfOptionItem{Tag: dt, Occurrence: occ[d]}
			if v, ok := c.attr(d, "IsChecked"); ok {
				item.HasChecked = true
				item.IsChecked = strings.EqualFold(v, "true")
			}
			if v, ok := c.attr(d, "None"); ok {
				item.IsNone = strings.EqualFold(v, "true")
			}
			for _, sub := range c.descendants(d) {
				st := c.doc.Elems[sub].Tag
				if c.doc.Elems[sub].Parent != d {
					continue
				}
				if st == "Name" {
					item.Name = c.text(sub)
					item.NameOccurrence = occ[sub]
				}
				if st == "Text" {
					item.Text = c.text(sub)
					item.TextOccurrence = occ[sub]
				}
			}
			page.Options = append(page.Options, item)
		}
		out = append(out, page)
	}
	return out
}

// SoftwarePackages 解析 Software 段
type ConfPackage struct {
	Option            string `json:"option"`
	Name              string `json:"name"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	Icon              string `json:"icon"`
	DefaultWebBrowser bool   `json:"defaultWebBrowser"`
}

// SoftwarePackages 返回 Software/Package 列表
func (c *Conf) SoftwarePackages() []ConfPackage {
	occ := c.occIndexes()
	var out []ConfPackage
	for i := range c.doc.Elems {
		if c.doc.Elems[i].Tag != "Package" {
			continue
		}
		p := ConfPackage{}
		if v, ok := c.attr(i, "Option"); ok {
			p.Option = v
		}
		if v, ok := c.attr(i, "DefaultWebBrowser"); ok {
			p.DefaultWebBrowser = strings.EqualFold(v, "true")
		}
		for _, sub := range c.descendants(i) {
			if c.doc.Elems[sub].Parent != i {
				continue
			}
			switch c.doc.Elems[sub].Tag {
			case "Name":
				p.Name = c.text(sub)
			case "Title":
				p.Title = c.text(sub)
			case "Description":
				p.Description = c.text(sub)
			case "Icon":
				p.Icon = c.text(sub)
			}
		}
		_ = occ
		out = append(out, p)
	}
	return out
}

// Value 取某标签首个元素的文本
func (c *Conf) Value(tag string) string {
	if i, err := c.elemIndex(tag, 0); err == nil {
		e := c.doc.Elems[i]
		if e.InnerEnd > e.InnerStart {
			return strings.TrimSpace(c.Text[e.InnerStart:e.InnerEnd])
		}
	}
	return ""
}

// Values 取某标签所有元素的文本
func (c *Conf) Values(tag string) []string {
	var out []string
	for i := range c.doc.Elems {
		if c.doc.Elems[i].Tag == tag {
			e := c.doc.Elems[i]
			if e.InnerEnd > e.InnerStart {
				out = append(out, strings.TrimSpace(c.Text[e.InnerStart:e.InnerEnd]))
			} else {
				out = append(out, "")
			}
		}
	}
	return out
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// SetText 定点替换元素文本
func (c *Conf) SetText(tag string, occurrence int, value string) error {
	idx, err := c.elemIndex(tag, occurrence)
	if err != nil {
		return err
	}
	e := c.doc.Elems[idx]
	if e.SelfClosing {
		return fmt.Errorf("<%s> 是自闭合标签，无法写入文本", tag)
	}
	esc := escapeXML(value)
	c.Text = c.Text[:e.InnerStart] + esc + c.Text[e.InnerEnd:]
	c.doc = scanXML(c.Text)
	return nil
}

// SetAttr 定点替换元素属性值
func (c *Conf) SetAttr(tag string, occurrence int, attr, value string) error {
	idx, err := c.elemIndex(tag, occurrence)
	if err != nil {
		return err
	}
	e := c.doc.Elems[idx]
	for _, a := range e.Attrs {
		if a.Name == attr {
			c.Text = c.Text[:a.Start] + value + c.Text[a.End:]
			c.doc = scanXML(c.Text)
			return nil
		}
	}
	return fmt.Errorf("<%s> 没有属性 %s", tag, attr)
}

// ConfSummary 用于界面展示的基本信息
type ConfSummary struct {
	Name             string   `json:"name"`
	Username         string   `json:"username"`
	Title            string   `json:"title"`
	Version          string   `json:"version"`
	UniqueId         string   `json:"uniqueId"`
	Description      string   `json:"description"`
	ShortDescription string   `json:"shortDescription"`
	Details          string   `json:"details"`
	ProductCode      string   `json:"productCode"`
	Git              string   `json:"git"`
	Website          string   `json:"website"`
	DonateLink       string   `json:"donateLink"`
	UpgradableFrom   string   `json:"upgradableFrom"`
	SupportedBuilds  []string `json:"supportedBuilds"`
	Requirements     []string `json:"requirements"`
}

// Summary 汇总基本信息
func (c *Conf) Summary() ConfSummary {
	return ConfSummary{
		Name:             c.Value("Name"),
		Username:         c.Value("Username"),
		Title:            c.Value("Title"),
		Version:          c.Value("Version"),
		UniqueId:         c.Value("UniqueId"),
		Description:      c.Value("Description"),
		ShortDescription: c.Value("ShortDescription"),
		Details:          c.Value("Details"),
		ProductCode:      c.Value("ProductCode"),
		Git:              c.Value("Git"),
		Website:          c.Value("Website"),
		DonateLink:       c.Value("DonateLink"),
		UpgradableFrom:   c.Value("UpgradableFrom"),
		SupportedBuilds:  c.Values("string"),
		Requirements:     c.Values("Requirement"),
	}
}

// OptionNames 提取 FeaturePages 中定义的所有选项名（用于校验 option 引用）
func (c *Conf) OptionNames() map[string]bool {
	out := map[string]bool{}
	for _, e := range c.doc.Elems {
		if e.Tag == "Package" {
			for _, a := range e.Attrs {
				if a.Name == "Option" {
					out[c.Text[a.Start:a.End]] = true
				}
			}
		}
	}
	for _, page := range c.OptionPages() {
		for _, o := range page.Options {
			if o.Name != "" {
				out[o.Name] = true
			}
		}
	}
	return out
}
