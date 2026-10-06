// Package tui renders a terminal UI showing batch processing progress. It
// consumes batch.ProgressEvent values published on a channel; it never
// decides anything about the pipeline itself, and internal/batch has no
// knowledge of this package.
package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"scribin/internal/batch"
)

var (
	styleHeader  = lipgloss.NewStyle().Bold(true)
	stylePending = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleActive  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	styleFooter  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
)

// stageLabels maps a batch.Stage to the text shown next to a video's name.
var stageLabels = map[batch.Stage]string{
	batch.StagePending:      "pending",
	batch.StageExtracting:   "extracting audio",
	batch.StageTranscribing: "transcribing",
	batch.StageDone:         "done",
	batch.StageSkipped:      "skipped",
	batch.StageError:        "error",
}

// stagePercent gives the row-level progress bar a rough position within the
// pipeline. There's no fine-grained progress from ffmpeg/whisper-cli itself,
// so this is a coarse, stage-based approximation, not a true percentage.
func stagePercent(stage batch.Stage) float64 {
	switch stage {
	case batch.StageExtracting:
		return 0.4
	case batch.StageTranscribing:
		return 0.8
	case batch.StageDone, batch.StageSkipped, batch.StageError:
		return 1.0
	default:
		return 0
	}
}

func styleForStage(stage batch.Stage) lipgloss.Style {
	switch stage {
	case batch.StageDone, batch.StageSkipped:
		return styleDone
	case batch.StageError:
		return styleError
	case batch.StageExtracting, batch.StageTranscribing:
		return styleActive
	default:
		return stylePending
	}
}

// videoRow is the UI's view of a single video's progress.
type videoRow struct {
	Name  string
	Stage batch.Stage
	Err   error
}

// progressEventMsg wraps a batch.ProgressEvent as a tea.Msg.
type progressEventMsg batch.ProgressEvent

// eventsClosedMsg is sent once the progress channel is drained and closed.
type eventsClosedMsg struct{}

// runFinishedMsg carries batch.Run's final result.
type runFinishedMsg struct {
	summary batch.Summary
	err     error
}

// Model is the Bubble Tea model for the batch progress screen.
type Model struct {
	rows  []videoRow
	index map[string]int

	rowBar     progress.Model
	overallBar progress.Model

	total     int
	succeeded int
	failed    int
	skipped   int

	start    time.Time
	done     bool
	finalErr error

	events <-chan batch.ProgressEvent
}

func newModel(videoNames []string, events <-chan batch.ProgressEvent) Model {
	rows := make([]videoRow, len(videoNames))
	index := make(map[string]int, len(videoNames))
	for i, name := range videoNames {
		rows[i] = videoRow{Name: name, Stage: batch.StagePending}
		index[name] = i
	}

	return Model{
		rows:       rows,
		index:      index,
		rowBar:     progress.New(progress.WithWidth(20), progress.WithoutPercentage()),
		overallBar: progress.New(progress.WithWidth(30)),
		start:      time.Now(),
		events:     events,
	}
}

func waitForEvent(events <-chan batch.ProgressEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return eventsClosedMsg{}
		}
		return progressEventMsg(ev)
	}
}

func (m Model) Init() tea.Cmd {
	return waitForEvent(m.events)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		if m.done {
			return m, tea.Quit
		}
		return m, nil

	case progressEventMsg:
		ev := batch.ProgressEvent(msg)
		i, ok := m.index[ev.VideoName]
		if !ok {
			i = len(m.rows)
			m.rows = append(m.rows, videoRow{Name: ev.VideoName})
			m.index[ev.VideoName] = i
		}
		m.rows[i].Stage = ev.Stage
		m.rows[i].Err = ev.Err

		switch ev.Stage {
		case batch.StagePending:
			m.total++
		case batch.StageDone:
			m.succeeded++
		case batch.StageError:
			m.failed++
		case batch.StageSkipped:
			m.skipped++
		}

		return m, waitForEvent(m.events)

	case eventsClosedMsg:
		// batch.Run closes the channel once it's done publishing; the
		// authoritative final summary/error arrives separately below.
		return m, nil

	case runFinishedMsg:
		m.done = true
		m.finalErr = msg.err
		return m, nil
	}

	return m, nil
}

func (m Model) View() tea.View {
	var b strings.Builder

	b.WriteString(styleHeader.Render("scribin — transcribing videos"))
	b.WriteString("\n\n")

	for _, row := range m.rows {
		label := styleForStage(row.Stage).Render(stageLabels[row.Stage])
		line := fmt.Sprintf("%-32s %s", row.Name, label)

		if row.Stage == batch.StageExtracting || row.Stage == batch.StageTranscribing {
			line += " " + m.rowBar.ViewAs(stagePercent(row.Stage))
		}
		if row.Stage == batch.StageError && row.Err != nil {
			line += styleError.Render(fmt.Sprintf(" (%v)", row.Err))
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	completed := m.succeeded + m.failed
	percent := 0.0
	if m.total > 0 {
		percent = float64(completed) / float64(m.total)
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Overall: %d/%d %s\n", completed, m.total, m.overallBar.ViewAs(percent)))

	if m.done {
		elapsed := time.Since(m.start).Round(time.Second)
		b.WriteString(fmt.Sprintf(
			"\nFinished in %s — %d succeeded, %d failed, %d skipped\n",
			elapsed, m.succeeded, m.failed, m.skipped,
		))
		if m.finalErr != nil {
			b.WriteString(styleError.Render(fmt.Sprintf("error: %v\n", m.finalErr)))
		}
		b.WriteString(styleFooter.Render("(press any key to exit)"))
		b.WriteString("\n")
	}

	view := tea.NewView(b.String())
	view.AltScreen = true
	return view
}

// Run shows the progress TUI while batch.Run processes cfg.InputDir in the
// background, and returns once processing is done and the user has
// dismissed the final summary (or quit early).
func Run(ctx context.Context, cfg batch.Config) (batch.Summary, error) {
	videos, err := batch.FindVideos(cfg.InputDir)
	if err != nil {
		return batch.Summary{}, err
	}
	names := make([]string, len(videos))
	for i, v := range videos {
		names[i] = filepath.Base(v)
	}

	// extract/transcribe/batch log via the standard "log" package, which
	// would otherwise corrupt the TUI's alternate screen. Redirect it to a
	// file for the duration of the program, and restore it afterward so
	// the caller's own log output (e.g. the final summary line) still
	// reaches the terminal normally.
	prevLogOutput := log.Writer()
	logFile, err := tea.LogToFile(filepath.Join(os.TempDir(), "scribin-tui.log"), "")
	if err != nil {
		return batch.Summary{}, fmt.Errorf("open tui log file: %w", err)
	}
	defer func() {
		logFile.Close()
		log.SetOutput(prevLogOutput)
	}()

	progressCh := make(chan batch.ProgressEvent, 64)
	cfg.Progress = progressCh

	model := newModel(names, progressCh)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	var summary batch.Summary
	var runErr error
	go func() {
		summary, runErr = batch.Run(ctx, cfg)
		program.Send(runFinishedMsg{summary: summary, err: runErr})
	}()

	if _, err := program.Run(); err != nil {
		return summary, err
	}

	return summary, runErr
}
