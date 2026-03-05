package ssh

import (
	"bufio"
	"context"
	"fmt"
	"time"

	"github.com/fluid-movement/log-tui/clog"
	"github.com/fluid-movement/log-tui/config"
	tea "charm.land/bubbletea/v2"
)

// LogLine is a single line received from a tail session.
type LogLine struct {
	Host     string
	Received time.Time
	Raw      string
	LineNum  int
}

// LogLineMsg is the Bubbletea message wrapping a LogLine.
type LogLineMsg LogLine

// HostStatusMsg notifies the UI of a status change on a specific host.
type HostStatusMsg struct {
	Host   string
	Status HostStatus
	Err    error
}

// reconnectMsg is an internal message that triggers a reconnect attempt.
type ReconnectMsg struct {
	Host    config.Host
	Attempt int
}

const tailCmdFmt = "tail -F -n 0 %s"
const grepCmdFmt = "grep -n -E %q %s"

// ListenForLog returns a Cmd that reads one LogLineMsg from ch.
// The caller MUST re-issue this after every received LogLineMsg.
func ListenForLog(ch chan LogLineMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// StartTail opens a tail -F session and streams lines into the returned channel.
// Returns a tea.Cmd (listenForLog) and the channel.
func StartTail(ctx context.Context, client *Client, filePath string, ch chan LogLineMsg) tea.Cmd {
	go func() {
		if err := streamTail(ctx, client, filePath, ch); err != nil {
			clog.Debug("tail ended", "host", client.Host.Name, "err", err)
		}
	}()
	return ListenForLog(ch)
}

func streamTail(ctx context.Context, client *Client, filePath string, ch chan LogLineMsg) error {
	sess, err := client.conn.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()

	cmd := fmt.Sprintf(tailCmdFmt, filePath)
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	if err := sess.Start(cmd); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	lineNum := 0
	for {
		select {
		case <-ctx.Done():
			sess.Signal(gosshSignalINT)
			return nil
		default:
		}
		if !scanner.Scan() {
			return scanner.Err()
		}
		lineNum++
		select {
		case ch <- LogLineMsg{
			Host:     client.Host.Name,
			Received: time.Now(),
			Raw:      scanner.Text(),
			LineNum:  lineNum,
		}:
		case <-ctx.Done():
			return nil
		}
	}
}

// StartGrep runs grep -n -E on the remote and streams results into ch.
func StartGrep(ctx context.Context, client *Client, pattern, filePath string, ch chan LogLineMsg) {
	go func() {
		sess, err := client.conn.NewSession()
		if err != nil {
			clog.Debug("grep session error", "err", err)
			return
		}
		defer sess.Close()

		cmd := fmt.Sprintf(grepCmdFmt, pattern, filePath)
		stdout, err := sess.StdoutPipe()
		if err != nil {
			return
		}
		if err := sess.Start(cmd); err != nil {
			return
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case ch <- LogLineMsg{
				Host:     client.Host.Name,
				Received: time.Now(),
				Raw:      scanner.Text(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()
}

// reconnectDelays holds the backoff durations for reconnect attempts.
var reconnectDelays = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	30 * time.Second, // cap at 30s for attempt 4+
}

func reconnectDelay(attempt int) time.Duration {
	if attempt < len(reconnectDelays) {
		return reconnectDelays[attempt]
	}
	return reconnectDelays[len(reconnectDelays)-1]
}

// ScheduleReconnect returns a Cmd that fires a ReconnectMsg after the appropriate delay.
func ScheduleReconnect(host config.Host, attempt int) tea.Cmd {
	return tea.Tick(reconnectDelay(attempt), func(time.Time) tea.Msg {
		return ReconnectMsg{Host: host, Attempt: attempt}
	})
}

// gosshSignalINT is the SSH signal for interrupt.
const gosshSignalINT = "INT"
