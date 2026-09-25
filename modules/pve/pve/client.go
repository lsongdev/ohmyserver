package pve

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client 是 Proxmox VE API 的 HTTP 客户端
type Client struct {
	Context    context.Context
	BaseURL    string
	Username   string
	Password   string
	HTTPClient *http.Client

	// 认证信息（由 Login 填充）
	Ticket    string
	CSRFToken string
}

// NewClient 创建 PVE 客户端
func NewClient(ctx context.Context, baseURL, username, password string, insecure bool) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	return &Client{
		Context:    ctx,
		BaseURL:    strings.TrimRight(baseURL, "/") + "/api2/json",
		Username:   username,
		Password:   password,
		HTTPClient: &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}
}

// Login 获取 PVE Ticket 和 CSRF Token
func (c *Client) Login() error {
	data := url.Values{}
	data.Set("username", c.Username)
	data.Set("password", c.Password)

	req, err := http.NewRequestWithContext(c.Context, "POST", c.BaseURL+"/access/ticket", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %s, body: %s", resp.Status, string(body))
	}

	var result struct {
		Data struct {
			Ticket              string `json:"ticket"`
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("unmarshal login response: %w", err)
	}

	c.Ticket = result.Data.Ticket
	c.CSRFToken = result.Data.CSRFPreventionToken
	return nil
}

// doRequest 发送带认证的 HTTP 请求
func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	if c.Ticket == "" {
		return nil, fmt.Errorf("not logged in")
	}

	req, err := http.NewRequestWithContext(c.Context, method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}

	// PVE 使用 Cookie: PVEAuthCookie=Ticket
	req.AddCookie(&http.Cookie{
		Name:  "PVEAuthCookie",
		Value: c.Ticket,
	})

	// POST/PUT/DELETE 需要 CSRF Token
	if method != "GET" && c.CSRFToken != "" {
		req.Header.Set("CSRFPreventionToken", c.CSRFToken)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	return c.HTTPClient.Do(req)
}

// CreateVMParams 创建虚拟机参数
type CreateVMParams struct {
	VMID    int    // 虚拟机 ID（必填）
	Name    string // 虚拟机名称
	Memory  int    // 内存 MB
	Cores   int    // CPU 核心数
	Sockets int    // CPU 插槽数
	CPU     string // CPU 类型，如 host
	Net0    string // 网络配置，如 virtio,bridge=vmbr0
	SCSI0   string // 磁盘配置，如 local-lvm:8
	OSType  string // 操作系统类型，如 l26
	IDE2    string // 光驱/ISO，如 local:iso/debian.iso,media=cdrom
	Agent   int    // QEMU Guest Agent，1=启用
	BIOS    string // seabios 或 ovmf
}

// CreateVM 在指定节点上创建虚拟机
func (c *Client) CreateVM(node string, params CreateVMParams) (string, error) {
	data := url.Values{}
	data.Set("vmid", fmt.Sprintf("%d", params.VMID))
	if params.Name != "" {
		data.Set("name", params.Name)
	}
	if params.Memory > 0 {
		data.Set("memory", fmt.Sprintf("%d", params.Memory))
	}
	if params.Cores > 0 {
		data.Set("cores", fmt.Sprintf("%d", params.Cores))
	}
	if params.Sockets > 0 {
		data.Set("sockets", fmt.Sprintf("%d", params.Sockets))
	}
	if params.CPU != "" {
		data.Set("cpu", params.CPU)
	}
	if params.Net0 != "" {
		data.Set("net0", params.Net0)
	}
	if params.SCSI0 != "" {
		data.Set("scsi0", params.SCSI0)
	}
	if params.OSType != "" {
		data.Set("ostype", params.OSType)
	}
	if params.IDE2 != "" {
		data.Set("ide2", params.IDE2)
	}
	if params.Agent > 0 {
		data.Set("agent", fmt.Sprintf("%d", params.Agent))
	}
	if params.BIOS != "" {
		data.Set("bios", params.BIOS)
	}

	path := fmt.Sprintf("/nodes/%s/qemu", node)
	resp, err := c.doRequest("POST", path, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create VM failed: %s, body: %s", resp.Status, string(body))
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body), nil // 有些接口返回的不是字符串
	}
	return result.Data, nil
}

// StartVM 启动虚拟机
func (c *Client) StartVM(node string, vmid int) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/start", node, vmid)
	resp, err := c.doRequest("POST", path, strings.NewReader(""))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("start VM failed: %s, body: %s", resp.Status, string(body))
	}
	return nil
}

// StopVM 停止虚拟机
func (c *Client) StopVM(node string, vmid int) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/stop", node, vmid)
	resp, err := c.doRequest("POST", path, strings.NewReader(""))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stop VM failed: %s, body: %s", resp.Status, string(body))
	}
	return nil
}

// GetVMs 列出节点上的虚拟机
func (c *Client) GetVMs(node string) ([]VMInfo, error) {
	path := fmt.Sprintf("/nodes/%s/qemu", node)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get VMs failed: %s, body: %s", resp.Status, string(body))
	}

	var result struct {
		Data []VMInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// VMInfo 虚拟机信息
type VMInfo struct {
	VMID     int    `json:"vmid"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	MaxMem   int64  `json:"maxmem"`
	MaxDisk  int64  `json:"maxdisk"`
	Uptime   int64  `json:"uptime"`
	CPUs     int    `json:"cpus"`
	Template int    `json:"template,omitempty"`
}

// GetNextVMID 获取下一个可用的 VMID
func (c *Client) GetNextVMID() (int, error) {
	resp, err := c.doRequest("GET", "/cluster/nextid", nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("get next vmid failed: %s", resp.Status)
	}

	var raw struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, err
	}

	var vmid int
	if err := json.Unmarshal(raw.Data, &vmid); err != nil {
		var str string
		if err2 := json.Unmarshal(raw.Data, &str); err2 != nil {
			return 0, err
		}
		vmid, err = strconv.Atoi(str)
		if err != nil {
			return 0, fmt.Errorf("parse vmid string: %w", err)
		}
	}
	return vmid, nil
}

// GetTemplates 列出节点上的模板 (template=1 的 VM)
func (c *Client) GetTemplates(node string) ([]VMInfo, error) {
	vms, err := c.GetVMs(node)
	if err != nil {
		return nil, err
	}
	var templates []VMInfo
	for _, vm := range vms {
		if vm.Template == 1 {
			templates = append(templates, vm)
		}
	}
	return templates, nil
}

// CloneVMParams 克隆模板参数
type CloneVMParams struct {
	NewID  int    // 新虚拟机 ID（必填）
	Name   string // 新虚拟机名称
	Target string // 目标节点（可选，跨节点克隆）
	Full   int    // 1=完整克隆, 0=链接克隆（默认 1）
}

// CloneVM 从模板克隆虚拟机
func (c *Client) CloneVM(node string, vmid int, params CloneVMParams) (string, error) {
	data := url.Values{}
	data.Set("newid", fmt.Sprintf("%d", params.NewID))
	if params.Name != "" {
		data.Set("name", params.Name)
	}
	if params.Target != "" {
		data.Set("target", params.Target)
	}
	if params.Full == 0 {
		data.Set("full", "0")
	} else {
		data.Set("full", "1")
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%d/clone", node, vmid)
	resp, err := c.doRequest("POST", path, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("clone VM failed: %s, body: %s", resp.Status, string(body))
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body), nil
	}
	return result.Data, nil
}

// GetVMConfig 获取虚拟机配置
func (c *Client) GetVMConfig(node string, vmid int) (map[string]interface{}, error) {
	path := fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid)
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get VM config failed: %s, body: %s", resp.Status, string(body))
	}

	var result struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// UpdateVMConfig 更新虚拟机配置 (CPU, memory, etc.)
func (c *Client) UpdateVMConfig(node string, vmid int, params map[string]string) error {
	data := url.Values{}
	for k, v := range params {
		data.Set(k, v)
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid)
	resp, err := c.doRequest("PUT", path, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update VM config failed: %s, body: %s", resp.Status, string(body))
	}
	return nil
}

// ResizeDiskVM 调整虚拟机磁盘大小 (增量添加)
func (c *Client) ResizeDiskVM(node string, vmid int, disk string, addGB int) error {
	data := url.Values{}
	data.Set("disk", disk)
	data.Set("size", fmt.Sprintf("+%dG", addGB))

	path := fmt.Sprintf("/nodes/%s/qemu/%d/resize", node, vmid)
	resp, err := c.doRequest("POST", path, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("resize disk failed: %s, body: %s", resp.Status, string(body))
	}
	return nil
}

// DeleteVM 删除虚拟机
func (c *Client) DeleteVM(node string, vmid int) error {
	path := fmt.Sprintf("/nodes/%s/qemu/%d", node, vmid)
	resp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete VM failed: %s, body: %s", resp.Status, string(body))
	}
	return nil
}

// GetNodes 获取集群节点列表
func (c *Client) GetNodes() ([]NodeInfo, error) {
	resp, err := c.doRequest("GET", "/nodes", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get nodes failed: %s", resp.Status)
	}

	var result struct {
		Data []NodeInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// NodeInfo 节点信息
type NodeInfo struct {
	Node   string  `json:"node"`
	Status string  `json:"status"`
	CPU    float64 `json:"cpu"`
	MaxCPU int     `json:"maxcpu"`
	Mem    int64   `json:"mem"`
	MaxMem int64   `json:"maxmem"`
	Uptime int64   `json:"uptime"`
}
