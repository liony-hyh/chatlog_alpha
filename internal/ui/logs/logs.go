package logs

import (
	"fmt"
	"time"

	"github.com/rivo/tview"
	"github.com/sjzar/chatlog/internal/ui/style"
)

type Logs struct {
	*tview.TextView
}

func New() *Logs {
	l := &Logs{
		TextView: tview.NewTextView(),
	}

	l.SetDynamicColors(true)
	l.SetScrollable(true)
	l.SetWrap(true)
	l.SetBorder(true)
	l.SetTitle(" 日志 ")
	l.SetBorderColor(style.BorderColor)
	l.SetBackgroundColor(style.BgColor)
	l.SetTextColor(style.FgColor)

	return l
}

func (l *Logs) AddLog(msg string) {
	fmt.Fprintf(l, "[%s]%s[white] %s\n", 
		time.Now().Format("15:04:05"), 
		"", 
		msg)
	l.ScrollToEnd()
}

func (l *Logs) Write(p []byte) (n int, err error) {
	l.AddLog(string(p))
	return len(p), nil
}
