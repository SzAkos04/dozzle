package container

// myGym fork: a container labelled dev.dozzle.update.script=<name> is not pulled and swapped by
// Dozzle. Updating it asks a runner on the host (UPDATE_RUNNER_URL, e.g. http://172.17.0.1:8085)
// to run the script of that name, such as the compose project's own server-update.sh, and streams
// its output into Dozzle's log. The runner decides which names exist; Dozzle never sends a command.
// Protocol: POST <runner>/run/<name> with "Authorization: Bearer <UPDATE_RUNNER_TOKEN>",
// answered by the script's output line by line and a last line "::exit <code>".

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// ScriptLabel names the host script that updates a container.
const ScriptLabel = "dev.dozzle.update.script"

// ScriptName is the script a container's labels choose, or "".
func ScriptName(labels map[string]string) string {
	return strings.TrimSpace(labels[ScriptLabel])
}

const scriptExitPrefix = "::exit "

// runScript runs the named script on the runner and reports it as an update. It closes ch.
func runScript(ctx context.Context, name string, ch chan<- UpdateProgress) (bool, error) {
	defer close(ch)
	send := func(p UpdateProgress) {
		select {
		case ch <- p:
		case <-ctx.Done():
		}
	}
	fail := func(err error) (bool, error) {
		send(UpdateProgress{Status: UpdateError, Error: err.Error()})
		return false, err
	}

	base := strings.TrimRight(os.Getenv("UPDATE_RUNNER_URL"), "/")
	if base == "" {
		return fail(fmt.Errorf("UPDATE_RUNNER_URL is not set, cannot run the %q update script", name))
	}
	send(UpdateProgress{Status: UpdateRecreating})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/run/"+url.PathEscape(name), nil)
	if err != nil {
		return fail(err)
	}
	if token := os.Getenv("UPDATE_RUNNER_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fail(fmt.Errorf("update runner: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail(fmt.Errorf("update runner answered %s for %q", resp.Status, name))
	}

	code, tail := -1, []string{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if rest, ok := strings.CutPrefix(line, scriptExitPrefix); ok {
			code, _ = strconv.Atoi(strings.TrimSpace(rest))
			continue
		}
		log.Info().Str("script", name).Msg(line)
		if tail = append(tail, line); len(tail) > 5 {
			tail = tail[1:]
		}
	}
	if code != 0 {
		// -1: the stream ended without the exit line, the script is probably still running on the host.
		msg := strings.Join(tail, "\n")
		if code < 0 {
			msg = "update runner closed the connection before the script finished\n" + msg
		}
		return fail(fmt.Errorf("update script %q failed (exit %d)\n%s", name, code, msg))
	}
	send(UpdateProgress{Status: UpdateDone})
	return true, nil
}
