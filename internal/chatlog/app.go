package chatlog

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/sjzar/chatlog/internal/chatlog/ctx"
	"github.com/sjzar/chatlog/internal/ui/dashboard"
	"github.com/sjzar/chatlog/internal/ui/footer"
	"github.com/sjzar/chatlog/internal/ui/form"
	"github.com/sjzar/chatlog/internal/ui/help"
	"github.com/sjzar/chatlog/internal/ui/layout"
	"github.com/sjzar/chatlog/internal/ui/logs"
	"github.com/sjzar/chatlog/internal/ui/menu"
	"github.com/sjzar/chatlog/internal/ui/sidebar"
	"github.com/sjzar/chatlog/internal/wechat"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	RefreshInterval = 1000 * time.Millisecond
)

type App struct {
	*tview.Application

	ctx         *ctx.Context
	m           *Manager
	stopRefresh chan struct{}

	// UI Components
	layout    *layout.Layout
	sidebar   *sidebar.Sidebar
	dashboard *dashboard.Dashboard
	logs      *logs.Logs
	footer    *footer.Footer

	// Page Managers
	rootPages    *tview.Pages // Handles Main Layout + Modals
	contentPages *tview.Pages // Handles Content (Dashboard, Actions, Settings, etc.)

	// Specific Pages
	actionsMenu *menu.Menu
	help        *help.Help
}

func NewApp(ctx *ctx.Context, m *Manager) *App {
	app := &App{
		ctx:          ctx,
		m:            m,
		Application:  tview.NewApplication(),
		rootPages:    tview.NewPages(),
		contentPages: tview.NewPages(),
		dashboard:    dashboard.New(),
		logs:         logs.New(),
		footer:       footer.New(),
		actionsMenu:  menu.New("操作菜单"),
		help:         help.New(),
	}

	// Initialize Sidebar
	app.sidebar = sidebar.New(app.onSidebarSelected)
	app.sidebar.AddItem("概览", '1')
	app.sidebar.AddItem("操作", '2')
	app.sidebar.AddItem("设置", '3')
	app.sidebar.AddItem("日志", '4')
	app.sidebar.AddItem("帮助", '5')

	// Initialize Layout
	app.layout = layout.New(app.sidebar, app.contentPages)

	// Build the main view with Footer
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(app.layout, 0, 1, true).
		AddItem(app.footer, 1, 1, false)

	app.rootPages.AddPage("main", flex, true, true)

	// Initialize Pages
	app.contentPages.AddPage("概览", app.dashboard, true, true)
	app.contentPages.AddPage("操作", app.actionsMenu, true, false)
	app.contentPages.AddPage("日志", app.logs, true, false)
	app.contentPages.AddPage("帮助", app.help, true, false)
	
	// Settings page will be dynamic (using form) but for now let's add a placeholder or the sub-menu logic
	app.initSettingsPage()

	app.initMenu()
	app.updateMenuItemsState()

	return app
}

func (a *App) Run() error {
	a.SetInputCapture(a.inputCapture)

	go a.refresh()

	if err := a.SetRoot(a.rootPages, true).EnableMouse(true).Run(); err != nil {
		return err
	}

	return nil
}

func (a *App) Stop() {
	if a.stopRefresh != nil {
		close(a.stopRefresh)
	}
	a.Application.Stop()
}

func (a *App) onSidebarSelected(index int, mainText string, secondaryText string, shortcut rune) {
	if a.contentPages.HasPage(mainText) {
		a.contentPages.SwitchToPage(mainText)
	}
}

func (a *App) initSettingsPage() {
	// For settings, we can reuse the Menu structure as a list of settings categories
	settingsMenu := menu.New("系统设置")
	
	settings := []settingItem{
		{
			name:        "设置 HTTP 服务地址",
			description: "配置 HTTP 服务监听的地址",
			action:      a.settingHTTPPort,
		},
		{
			name:        "设置工作目录",
			description: "配置数据解密后的存储目录",
			action:      a.settingWorkDir,
		},
		{
			name:        "设置数据密钥",
			description: "配置数据解密密钥",
			action:      a.settingDataKey,
		},
		{
			name:        "设置图片密钥",
			description: "配置图片解密密钥",
			action:      a.settingImgKey,
		},
		{
			name:        "设置数据目录",
			description: "配置微信数据文件所在目录",
			action:      a.settingDataDir,
		},
	}

	for idx, setting := range settings {
		item := &menu.Item{
			Index:       idx + 1,
			Name:        setting.name,
			Description: setting.description,
			Selected: func(action func()) func(*menu.Item) {
				return func(*menu.Item) {
					action()
				}
			}(setting.action),
		}
		settingsMenu.AddItem(item)
	}
	
	a.contentPages.AddPage("设置", settingsMenu, true, false)
}

func (a *App) updateMenuItemsState() {
	for _, item := range a.actionsMenu.GetItems() {
		// Auto Decrypt
		if item.Index == 6 {
			if a.ctx.AutoDecrypt {
				item.Name = "停止自动解密"
				item.Description = "停止监控数据目录更新，不再自动解密新增数据"
			} else {
				item.Name = "开启自动解密"
				item.Description = "监控数据目录更新，自动解密新增数据"
			}
		}

		// HTTP Service
		if item.Index == 5 {
			if a.ctx.HTTPEnabled {
				item.Name = "停止 HTTP 服务"
				item.Description = "停止本地 HTTP & MCP 服务器"
			} else {
				item.Name = "启动 HTTP 服务"
				item.Description = "启动本地 HTTP & MCP 服务器"
			}
		}
	}
}

func (a *App) refresh() {
	tick := time.NewTicker(RefreshInterval)
	defer tick.Stop()

	for {
		select {
		case <-a.stopRefresh:
			return
		case <-tick.C:
			// Auto-detect account if nil
			if a.ctx.Current == nil {
				instances := a.m.wechat.GetWeChatInstances()
				if len(instances) > 0 {
					a.ctx.SwitchCurrent(instances[0])
					a.logs.AddLog(fmt.Sprintf("检测到微信进程，PID: %d，已设置为当前账号", instances[0].PID))
				}
			}

			// Refresh account status
			if a.ctx.Current != nil {
				originalName := a.ctx.Current.Name
				a.ctx.Current.RefreshStatus()
				if a.ctx.Current.Name != originalName {
					a.ctx.SwitchCurrent(a.ctx.Current)
				} else {
					a.ctx.Refresh()
				}
			}

			if a.ctx.AutoDecrypt || a.ctx.HTTPEnabled {
				a.m.RefreshSession()
			}

			// Update Dashboard
			dashboardData := map[string]string{
				"Account":      a.ctx.Account,
				"PID":          fmt.Sprintf("%d", a.ctx.PID),
				"Status":       a.ctx.Status,
				"ExePath":      a.ctx.ExePath,
				"Platform":     a.ctx.Platform,
				"Version":      a.ctx.FullVersion,
				"Session":      "",
				"Data Key":     a.ctx.DataKey,
				"Image Key":    a.ctx.ImgKey,
				"Data Usage":   a.ctx.DataUsage,
				"Data Dir":     a.ctx.DataDir,
				"Work Usage":   a.ctx.WorkUsage,
				"Work Dir":     a.ctx.WorkDir,
				"HTTP Server":  "[未启动]",
				"Auto Decrypt": "[未开启]",
			}

			if a.ctx.LastSession.Unix() > 1000000000 {
				dashboardData["Session"] = a.ctx.LastSession.Format("2006-01-02 15:04:05")
			}
			if a.ctx.HTTPEnabled {
				dashboardData["HTTP Server"] = fmt.Sprintf("[green][已启动][white] [%s]", a.ctx.HTTPAddr)
			}
			if a.ctx.AutoDecrypt {
				dashboardData["Auto Decrypt"] = "[green][已开启][white]"
			}
			a.dashboard.Update(dashboardData)

			// Update latest message in footer
			if session, err := a.m.GetLatestSession(); err == nil && session != nil {
				sender := session.NickName
				if sender == "" {
					sender = session.UserName
				}
				a.footer.UpdateLatestMessage(sender, session.NTime.Format("15:04:05"), session.Content)
			}

			a.Draw()
		}
	}
}

func (a *App) inputCapture(event *tcell.EventKey) *tcell.EventKey {
	// If a modal is open (like settings form), let it handle input
	// Simple check: if rootPages front page is not "main"
	name, _ := a.rootPages.GetFrontPage()
	if name != "main" {
		return event
	}

	switch event.Key() {
	case tcell.KeyCtrlC:
		a.Stop()
	case tcell.KeyTab:
		// Switch focus between sidebar and content
		if a.sidebar.HasFocus() {
			_, item := a.contentPages.GetFrontPage()
			if item != nil {
				a.SetFocus(item)
			}
		} else {
			a.SetFocus(a.sidebar)
		}
	}

	return event
}

func (a *App) initMenu() {
	getDataKey := &menu.Item{
		Index:       2,
		Name:        "获取图片密钥",
		Description: "扫描内存获取图片密钥(需微信V4)",
		Selected: func(i *menu.Item) {
			a.logs.AddLog("开始扫描内存获取图片密钥...")
			modal := tview.NewModal()
			modal.SetText("正在扫描内存获取图片密钥...\n请确保微信已登录并浏览过图片")
			a.rootPages.AddPage("modal", modal, true, true)
			a.SetFocus(modal)

			go func() {
				err := a.m.GetImageKey()

				a.QueueUpdateDraw(func() {
					if err != nil {
						a.logs.AddLog("获取图片密钥失败: " + err.Error())
						modal.SetText("获取图片密钥失败: " + err.Error())
					} else {
						a.logs.AddLog("获取图片密钥成功")
						modal.SetText("获取图片密钥成功")
					}

					modal.AddButtons([]string{"OK"})
					modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						a.rootPages.RemovePage("modal")
					})
					a.SetFocus(modal)
				})
			}()
		},
	}

	restartAndGetDataKey := &menu.Item{
		Index:       3,
		Name:        "重启并获取密钥",
		Description: "结束当前微信进程，重启后获取密钥",
		Selected: func(i *menu.Item) {
			a.logs.AddLog("准备重启微信获取密钥...")
			modal := tview.NewModal().SetText("正在准备重启微信...")
			a.rootPages.AddPage("modal", modal, true, true)
			a.SetFocus(modal)

			go func() {
				onStatus := func(msg string) {
					a.QueueUpdateDraw(func() {
						modal.SetText(msg)
						a.logs.AddLog(msg)
					})
				}

				err := a.m.RestartAndGetDataKey(onStatus)

				a.QueueUpdateDraw(func() {
					if err != nil {
						a.logs.AddLog("重启获取密钥失败: " + err.Error())
						modal.SetText("操作失败: " + err.Error())
					} else {
						a.logs.AddLog("重启获取密钥成功")
						modal.SetText("操作成功，请检查密钥是否已更新")
					}

					modal.AddButtons([]string{"OK"})
					modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						a.rootPages.RemovePage("modal")
					})
					a.SetFocus(modal)
				})
			}()
		},
	}

	decryptData := &menu.Item{
		Index:       4,
		Name:        "解密数据",
		Description: "解密数据文件",
		Selected: func(i *menu.Item) {
			a.logs.AddLog("开始解密数据...")
			modal := tview.NewModal().SetText("解密中...")
			a.rootPages.AddPage("modal", modal, true, true)
			a.SetFocus(modal)

			go func() {
				err := a.m.DecryptDBFiles()

				a.QueueUpdateDraw(func() {
					if err != nil {
						a.logs.AddLog("解密失败: " + err.Error())
						modal.SetText("解密失败: " + err.Error())
					} else {
						a.logs.AddLog("解密数据成功")
						modal.SetText("解密数据成功")
					}

					modal.AddButtons([]string{"OK"})
					modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						a.rootPages.RemovePage("modal")
					})
					a.SetFocus(modal)
				})
			}()
		},
	}

	httpServer := &menu.Item{
		Index:       5,
		Name:        "启动 HTTP 服务",
		Description: "启动本地 HTTP & MCP 服务器",
		Selected: func(i *menu.Item) {
			modal := tview.NewModal()

			if !a.ctx.HTTPEnabled {
				a.logs.AddLog("正在启动 HTTP 服务...")
				modal.SetText("正在启动 HTTP 服务...")
				a.rootPages.AddPage("modal", modal, true, true)
				a.SetFocus(modal)

				go func() {
					err := a.m.StartService()

					a.QueueUpdateDraw(func() {
						if err != nil {
							a.logs.AddLog("启动 HTTP 服务失败: " + err.Error())
							modal.SetText("启动 HTTP 服务失败: " + err.Error())
						} else {
							a.logs.AddLog("已启动 HTTP 服务")
							modal.SetText("已启动 HTTP 服务")
						}

						a.updateMenuItemsState()

						modal.AddButtons([]string{"OK"})
						modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.rootPages.RemovePage("modal")
						})
						a.SetFocus(modal)
					})
				}()
			} else {
				a.logs.AddLog("正在停止 HTTP 服务...")
				modal.SetText("正在停止 HTTP 服务...")
				a.rootPages.AddPage("modal", modal, true, true)
				a.SetFocus(modal)

				go func() {
					err := a.m.StopService()

					a.QueueUpdateDraw(func() {
						if err != nil {
							a.logs.AddLog("停止 HTTP 服务失败: " + err.Error())
							modal.SetText("停止 HTTP 服务失败: " + err.Error())
						} else {
							a.logs.AddLog("已停止 HTTP 服务")
							modal.SetText("已停止 HTTP 服务")
						}

						a.updateMenuItemsState()

						modal.AddButtons([]string{"OK"})
						modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.rootPages.RemovePage("modal")
						})
						a.SetFocus(modal)
					})
				}()
			}
		},
	}

	autoDecrypt := &menu.Item{
		Index:       6,
		Name:        "开启自动解密",
		Description: "自动解密新增的数据文件",
		Selected: func(i *menu.Item) {
			modal := tview.NewModal()

			if !a.ctx.AutoDecrypt {
				a.logs.AddLog("开启自动解密...")
				modal.SetText("正在开启自动解密...")
				a.rootPages.AddPage("modal", modal, true, true)
				a.SetFocus(modal)

				go func() {
					err := a.m.StartAutoDecrypt()

					a.QueueUpdateDraw(func() {
						if err != nil {
							a.logs.AddLog("开启自动解密失败: " + err.Error())
							modal.SetText("开启自动解密失败: " + err.Error())
						} else {
							a.logs.AddLog("已开启自动解密")
							modal.SetText("已开启自动解密")
						}

						a.updateMenuItemsState()
						modal.AddButtons([]string{"OK"})
						modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.rootPages.RemovePage("modal")
						})
						a.SetFocus(modal)
					})
				}()
			} else {
				a.logs.AddLog("停止自动解密...")
				modal.SetText("正在停止自动解密...")
				a.rootPages.AddPage("modal", modal, true, true)
				a.SetFocus(modal)

				go func() {
					err := a.m.StopAutoDecrypt()

					a.QueueUpdateDraw(func() {
						if err != nil {
							a.logs.AddLog("停止自动解密失败: " + err.Error())
							modal.SetText("停止自动解密失败: " + err.Error())
						} else {
							a.logs.AddLog("已停止自动解密")
							modal.SetText("已停止自动解密")
						}

						a.updateMenuItemsState()
						modal.AddButtons([]string{"OK"})
						modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.rootPages.RemovePage("modal")
						})
						a.SetFocus(modal)
					})
				}()
			}
		},
	}

	selectAccount := &menu.Item{
		Index:       8,
		Name:        "切换账号",
		Description: "切换当前操作的账号，可以选择进程或历史账号",
		Selected:    a.selectAccountSelected,
	}

	mcpSubscriptions := &menu.Item{
		Index:       9,
		Name:        "查看 MCP 订阅",
		Description: "查看当前正在订阅的实时消息流及推送状态",
		Selected: func(i *menu.Item) {
			subs := a.m.GetMCPSubscriptions()
			lastPushTime, lastPushTalker := a.m.GetMCPStatus()
			status := "[red]未启动[white]"
			if a.ctx.HTTPEnabled {
				status = "[green]运行中[white]"
			}

			text := fmt.Sprintf("推送服务状态: %s\n", status)
			text += fmt.Sprintf("推送服务地址: [cyan]%s[white]\n", a.ctx.HTTPAddr)
			if !lastPushTime.IsZero() {
				text += fmt.Sprintf("最近推送时间: [green]%s[white] (%s)\n", lastPushTime.Format("15:04:05"), lastPushTalker)
			} else {
				text += "最近推送时间: [gray]暂无推送[white]\n"
			}
			text += "[yellow]订阅信息已持久化保存到本地[white]\n\n"

			if len(subs) == 0 {
				text += "当前无活跃订阅。"
			} else {
				text += "活跃订阅列表:\n"
				for _, sub := range subs {
					statusStr := "[gray]等待中[white]"
					if sub.LastStatus == "Success" {
						statusStr = "[green]成功[white]"
					} else if sub.LastStatus == "Failed" || sub.LastStatus == "Error" {
						statusStr = fmt.Sprintf("[red]失败: %s[white]", sub.LastError)
					}
					text += fmt.Sprintf("- %s\n  状态: %s\n  推送地址: %s\n  订阅时间: %s\n", sub.Talker, statusStr, sub.WebhookURL, sub.LastTime.Format("2006-01-02 15:04:05"))
				}
			}

			modal := tview.NewModal().
				SetText(text).
				AddButtons([]string{"返回"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					a.rootPages.RemovePage("modal")
				})
			a.rootPages.AddPage("modal", modal, true, true)
			a.SetFocus(modal)
		},
	}

	a.actionsMenu.AddItem(getDataKey)
	a.actionsMenu.AddItem(restartAndGetDataKey)
	a.actionsMenu.AddItem(decryptData)
	a.actionsMenu.AddItem(httpServer)
	a.actionsMenu.AddItem(autoDecrypt)
	a.actionsMenu.AddItem(selectAccount)
	a.actionsMenu.AddItem(mcpSubscriptions)

	a.actionsMenu.AddItem(&menu.Item{
		Index:       10,
		Name:        "退出",
		Description: "退出程序",
		Selected: func(i *menu.Item) {
			a.Stop()
		},
	})
}

// settingItem represents a setting item
type settingItem struct {
	name        string
	description string
	action      func()
}

// settingHTTPPort Sets HTTP Port
func (a *App) settingHTTPPort() {
	formView := form.NewForm("设置 HTTP 地址")
	tempHTTPAddr := a.ctx.HTTPAddr

	formView.AddInputField("地址", tempHTTPAddr, 0, nil, func(text string) {
		tempHTTPAddr = text
	})

	formView.AddButton("保存", func() {
		a.m.SetHTTPAddr(tempHTTPAddr)
		a.rootPages.RemovePage("form")
		a.showInfo("HTTP 地址已设置为 " + a.ctx.HTTPAddr)
	})

	formView.AddButton("取消", func() {
		a.rootPages.RemovePage("form")
	})

	a.rootPages.AddPage("form", formView, true, true)
	a.SetFocus(formView)
}

// settingWorkDir Sets Work Dir
func (a *App) settingWorkDir() {
	formView := form.NewForm("设置工作目录")
	tempWorkDir := a.ctx.WorkDir

	formView.AddInputField("工作目录", tempWorkDir, 0, nil, func(text string) {
		tempWorkDir = text
	})

	formView.AddButton("保存", func() {
		a.ctx.SetWorkDir(tempWorkDir)
		a.rootPages.RemovePage("form")
		a.showInfo("工作目录已设置为 " + a.ctx.WorkDir)
	})

	formView.AddButton("取消", func() {
		a.rootPages.RemovePage("form")
	})

	a.rootPages.AddPage("form", formView, true, true)
	a.SetFocus(formView)
}

// settingDataKey Sets Data Key
func (a *App) settingDataKey() {
	formView := form.NewForm("设置数据密钥")
	tempDataKey := a.ctx.DataKey

	formView.AddInputField("数据密钥", tempDataKey, 0, nil, func(text string) {
		tempDataKey = text
	})

	formView.AddButton("保存", func() {
		a.ctx.DataKey = tempDataKey
		a.rootPages.RemovePage("form")
		a.showInfo("数据密钥已设置")
	})

	formView.AddButton("取消", func() {
		a.rootPages.RemovePage("form")
	})

	a.rootPages.AddPage("form", formView, true, true)
	a.SetFocus(formView)
}

// settingImgKey Sets Image Key
func (a *App) settingImgKey() {
	formView := form.NewForm("设置图片密钥")
	tempImgKey := a.ctx.ImgKey

	formView.AddInputField("图片密钥", tempImgKey, 0, nil, func(text string) {
		tempImgKey = text
	})

	formView.AddButton("保存", func() {
		a.ctx.SetImgKey(tempImgKey)
		a.rootPages.RemovePage("form")
		a.showInfo("图片密钥已设置")
	})

	formView.AddButton("取消", func() {
		a.rootPages.RemovePage("form")
	})

	a.rootPages.AddPage("form", formView, true, true)
	a.SetFocus(formView)
}

// settingDataDir Sets Data Dir
func (a *App) settingDataDir() {
	formView := form.NewForm("设置数据目录")
	tempDataDir := a.ctx.DataDir

	formView.AddInputField("数据目录", tempDataDir, 0, nil, func(text string) {
		tempDataDir = text
	})

	formView.AddButton("保存", func() {
		a.ctx.DataDir = tempDataDir
		a.rootPages.RemovePage("form")
		a.showInfo("数据目录已设置为 " + a.ctx.DataDir)
	})

	formView.AddButton("取消", func() {
		a.rootPages.RemovePage("form")
	})

	a.rootPages.AddPage("form", formView, true, true)
	a.SetFocus(formView)
}

// selectAccountSelected Handles account switch selection
func (a *App) selectAccountSelected(i *menu.Item) {
	// Create sub-menu for account selection
	subMenu := menu.NewSubMenu("切换账号")

	// Add instances
	instances := a.m.wechat.GetWeChatInstances()
	if len(instances) > 0 {
		subMenu.AddItem(&menu.Item{Index: 0, Name: "--- 微信进程 ---", Description: "", Hidden: false, Selected: nil})

		for idx, instance := range instances {
			description := fmt.Sprintf("版本: %s 目录: %s", instance.FullVersion, instance.DataDir)
			name := fmt.Sprintf("%s [%d]", instance.Name, instance.PID)
			if a.ctx.Current != nil && a.ctx.Current.PID == instance.PID {
				name = name + " [当前]"
			}

			instanceItem := &menu.Item{
				Index:       idx + 1,
				Name:        name,
				Description: description,
				Hidden:      false,
				Selected: func(instance *wechat.Account) func(*menu.Item) {
					return func(*menu.Item) {
						if a.ctx.Current != nil && a.ctx.Current.PID == instance.PID {
							a.rootPages.RemovePage("submenu")
							a.showInfo("已经是当前账号")
							return
						}

						modal := tview.NewModal().SetText("正在切换账号...")
						a.rootPages.AddPage("modal", modal, true, true)
						a.SetFocus(modal)

						go func() {
							err := a.m.Switch(instance, "")
							a.QueueUpdateDraw(func() {
								a.rootPages.RemovePage("modal")
								a.rootPages.RemovePage("submenu")
								if err != nil {
									a.showError(fmt.Errorf("切换账号失败: %v", err))
								} else {
									a.showInfo("切换账号成功")
									a.updateMenuItemsState()
								}
							})
						}()
					}
				}(instance),
			}
			subMenu.AddItem(instanceItem)
		}
	}

	// Add History
	if len(a.ctx.History) > 0 {
		subMenu.AddItem(&menu.Item{Index: 100, Name: "--- 历史账号 ---", Description: "", Hidden: false, Selected: nil})
		idx := 101
		for account, hist := range a.ctx.History {
			description := fmt.Sprintf("版本: %s 目录: %s", hist.FullVersion, hist.DataDir)
			name := account
			if name == "" {
				name = filepath.Base(hist.DataDir)
			}
			if a.ctx.DataDir == hist.DataDir {
				name = name + " [当前]"
			}

			histItem := &menu.Item{
				Index:       idx,
				Name:        name,
				Description: description,
				Hidden:      false,
				Selected: func(account string) func(*menu.Item) {
					return func(*menu.Item) {
						if a.ctx.Current != nil && a.ctx.DataDir == a.ctx.History[account].DataDir {
							a.rootPages.RemovePage("submenu")
							a.showInfo("已经是当前账号")
							return
						}

						modal := tview.NewModal().SetText("正在切换账号...")
						a.rootPages.AddPage("modal", modal, true, true)
						a.SetFocus(modal)

						go func() {
							err := a.m.Switch(nil, account)
							a.QueueUpdateDraw(func() {
								a.rootPages.RemovePage("modal")
								a.rootPages.RemovePage("submenu")
								if err != nil {
									a.showError(fmt.Errorf("切换账号失败: %v", err))
								} else {
									a.showInfo("切换账号成功")
									a.updateMenuItemsState()
								}
							})
						}()
					}
				}(account),
			}
			idx++
			subMenu.AddItem(histItem)
		}
	}

	if len(a.ctx.History) == 0 && len(instances) == 0 {
		subMenu.AddItem(&menu.Item{Index: 1, Name: "无可用账号", Description: "未检测到微信进程或历史账号", Hidden: false, Selected: nil})
	}

	a.rootPages.AddPage("submenu", subMenu, true, true)
	a.SetFocus(subMenu)
}

// showModal Shows a modal
func (a *App) showModal(text string, buttons []string, doneFunc func(buttonIndex int, buttonLabel string)) {
	modal := tview.NewModal().
		SetText(text).
		AddButtons(buttons).
		SetDoneFunc(doneFunc)

	a.rootPages.AddPage("modal", modal, true, true)
	a.SetFocus(modal)
}

// showError Shows an error modal
func (a *App) showError(err error) {
	a.showModal(err.Error(), []string{"OK"}, func(buttonIndex int, buttonLabel string) {
		a.rootPages.RemovePage("modal")
	})
}

// showInfo Shows an info modal
func (a *App) showInfo(text string) {
	a.showModal(text, []string{"OK"}, func(buttonIndex int, buttonLabel string) {
		a.rootPages.RemovePage("modal")
	})
}