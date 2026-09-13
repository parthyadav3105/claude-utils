package main

import (
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

type Task struct {
	ID        int               `json:"-"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Owner     string            `json:"owner"`
	BlockedBy []int             `json:"blocked_by"`
	Created   time.Time         `json:"created"`
	Done      time.Time         `json:"done"`
	Body      string            `json:"body"`
	Project   string            `json:"-"`
}

type content struct {
	labels    []string
	blockedBy []string
	file      string
}

const whenToUse = `As a coding agent, use it when you plan larger work, or have several items to
complete, or the user asks for something new while you are busy with a task.`

const noTasks = `No pending tasks.

As a coding agent, use task cli when you plan larger work, or have several
items to complete, or the user asks for something new while you are busy with
a task. Run 'task --help' before you start using it.
`

func main() {
	root := &cobra.Command{
		Use: "task",
		Long: "Task Management Tool for AI Coding Agent\n\n" + whenToUse + `

Each larger piece of work becomes a task that you pull and finish one at a time.
Create a task with 'task create'. Work only on tasks you created or that the
user asks you to do, and before you start working on a task, claim it with
'task edit ID --set-owner OWNER', where OWNER is the name other agents use to
reach you. To hand a task to a subagent, set its owner to the subagent's name
and give the subagent the task ID. A subagent works on the task it was given
and does not create tasks of its own.

When a task has to wait for other tasks, pass their IDs to 'task create -b' or
'task edit -b'. Do not start a task while 'task list' shows it as blocked.

Read a task with 'task view', change it with 'task edit', and finish it with
'task done'. To link tasks, refer to one as #ID in another task's description.
In commands, type the ID as a bare number, as in 'task view 12'.

Install: ` + link("https://github.com/parthyadav3105/claude-utils/tree/main/tasks", "github.com/parthyadav3105/claude-utils/tasks"),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
		},
	}
	root.AddCommand(createCmd(), listCmd(), viewCmd(), editCmd(), doneCmd(), deleteCmd(), setupCmd(), uninstallCmd(), hookCmd(), statuslineCmd())
	if root.Execute() != nil {
		os.Exit(1)
	}
}

func createCmd() *cobra.Command {
	var c content
	return c.flags(&cobra.Command{
		Use:   "create NAME",
		Short: "Create a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := project()
			if err != nil {
				return err
			}
			if err := singleLine("NAME", args[0]); err != nil {
				return err
			}
			t := Task{Name: args[0], Created: time.Now()}
			if err := c.apply(dir, &t); err != nil {
				return err
			}
			if err := create(dir, &t); err != nil {
				return err
			}
			fmt.Printf("#%d created\n", t.ID)
			return nil
		},
	})
}

func editCmd() *cobra.Command {
	var c content
	var name, owner string
	cmd := &cobra.Command{
		Use:   "edit ID",
		Short: "Change a task's name, owner, labels, blockers or body",
		Example: `  task edit 12 --set-owner reviewer
  task edit 12 --name "Fix login redirect"
  task edit 12 -l priority=high -l wip-
  task edit 12 -b 4 -b 3-
  task edit 12 -f - < notes.md`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeID,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().NFlag() == 0 {
				return errors.New("nothing to change: pass --name, --set-owner, -l, -b or -f")
			}
			dir, t, err := find(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("name") {
				if err := singleLine("NAME", name); err != nil {
					return err
				}
				t.Name = name
			}
			if cmd.Flags().Changed("set-owner") {
				if err := singleLine("OWNER", owner); err != nil && owner != "" {
					return err
				}
				t.Owner = owner
			}
			if err := c.apply(dir, &t); err != nil {
				return err
			}
			if err := save(dir, t); err != nil {
				return err
			}
			fmt.Printf("#%d edited\n", t.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "replace the name")
	cmd.Flags().StringVar(&owner, "set-owner", "", "replace the owner, or clear it with ''")
	cmd.Flags().StringSliceVarP(&c.labels, "label", "l", nil, "add or change a label as KEY=VALUE, or remove one as KEY-, repeated or comma-separated")
	cmd.Flags().StringSliceVarP(&c.blockedBy, "blocked-by", "b", nil, "add a blocker as ID, or remove one as ID-, repeated or comma-separated")
	cmd.Flags().StringVarP(&c.file, "file", "f", "", "replace the body with FILE, or with stdin when -")
	cmd.RegisterFlagCompletionFunc("label", completeLabels)
	cmd.RegisterFlagCompletionFunc("blocked-by", completeIDs)
	return cmd
}

func (c *content) flags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().StringSliceVarP(&c.labels, "label", "l", nil, "label as KEY=VALUE, repeated or comma-separated")
	cmd.Flags().StringSliceVarP(&c.blockedBy, "blocked-by", "b", nil, "IDs of tasks that must be done first, repeated or comma-separated")
	cmd.Flags().StringVarP(&c.file, "file", "f", "", "read the body from FILE, or from stdin with -")
	cmd.RegisterFlagCompletionFunc("label", completeLabels)
	cmd.RegisterFlagCompletionFunc("blocked-by", completeIDs)
	return cmd
}

func (c *content) apply(dir string, t *Task) error {
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	for _, l := range c.labels {
		if k, remove := removal(l); remove {
			if _, ok := t.Labels[k]; !ok {
				return fmt.Errorf("no label %q to remove", k)
			}
			delete(t.Labels, k)
			continue
		}
		k, v, err := pair(l, "=")
		if err != nil {
			return err
		}
		t.Labels[k] = v
	}
	for _, b := range c.blockedBy {
		s, remove := removal(b)
		id, err := parseID(s)
		if err != nil {
			return err
		}
		if remove {
			i := slices.Index(t.BlockedBy, id)
			if i < 0 {
				return fmt.Errorf("#%d is not a blocker to remove", id)
			}
			t.BlockedBy = slices.Delete(t.BlockedBy, i, i+1)
			continue
		}
		if _, err := load(dir, id); err != nil {
			return err
		}
		if !slices.Contains(t.BlockedBy, id) {
			t.BlockedBy = append(t.BlockedBy, id)
		}
	}
	if c.file == "" {
		return nil
	}
	var body []byte
	var err error
	if c.file == "-" {
		body, err = io.ReadAll(os.Stdin)
	} else {
		body, err = os.ReadFile(c.file)
	}
	t.Body = string(body)
	return err
}

func removal(s string) (string, bool) {
	if strings.Contains(s, "=") {
		return s, false
	}
	return strings.CutSuffix(s, "-")
}

func listCmd() *cobra.Command {
	var all, global bool
	var selectors []string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List open tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showList(global, all, selectors, limit)
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "A", false, "include done tasks")
	cmd.Flags().BoolVar(&global, "global", false, "list tasks from every project")
	cmd.Flags().IntVar(&limit, "limit", 0, "show at most N tasks")
	cmd.Flags().StringSliceVarP(&selectors, "selector", "l", nil, "filter by key=value or key!=value, comma-separated")
	cmd.RegisterFlagCompletionFunc("selector", completeLabels)
	return cmd
}

func showList(global, all bool, selectors []string, limit int) error {
	var excluded []func(Task) bool
	for _, s := range selectors {
		op := "="
		if strings.Contains(s, "!=") {
			op = "!="
		}
		k, v, err := pair(s, op)
		if err != nil {
			return err
		}
		excluded = append(excluded, func(t Task) bool { return (t.Labels[k] == v) != (op == "=") })
	}
	dir, err := project()
	if err != nil {
		return err
	}
	dirs := []string{dir}
	if global {
		if dirs, err = projects(); err != nil {
			return err
		}
	}
	var rows []string
	for _, d := range dirs {
		tasks, err := loadAll(d)
		if err != nil {
			return err
		}
		open := openIDs(tasks)
		for _, t := range tasks {
			if !all && !open[t.ID] || slices.ContainsFunc(excluded, func(exclude func(Task) bool) bool { return exclude(t) }) {
				continue
			}
			row := fmt.Sprintf("#%d\t%s\t%s\t%d\t%s\t%s\t%s", t.ID, t.Name, ago(t.Created), len(strings.Fields(t.Body)),
				t.Owner, blockers(t, open), labelList(t.Labels, ","))
			if all {
				row += "\t"
				if !t.Done.IsZero() {
					row += ago(t.Done)
				}
			}
			if global {
				row += "\t" + t.Project
			}
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		fmt.Print(noTasks)
		return nil
	}
	more := 0
	if limit > 0 && len(rows) > limit {
		more = len(rows) - limit
		rows = rows[:limit]
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	header := "ID\tNAME\tAGE\tWORDS\tOWNER\tBLOCKED BY\tLABELS"
	if all {
		header += "\tDONE"
	}
	if global {
		header += "\tPROJECT"
	}
	fmt.Fprintln(w, header)
	for _, row := range rows {
		fmt.Fprintln(w, row)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if more > 0 {
		fmt.Printf("... and %d more. Run 'task list' to see all.\n", more)
	}
	return nil
}

func viewCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "view ID",
		Short:             "Show a task and its body",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeID,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, t, err := find(args[0])
			if err != nil {
				return err
			}
			tasks, err := loadAll(dir)
			if err != nil {
				return err
			}
			done := "-"
			if !t.Done.IsZero() {
				done = stamp(t.Done)
			}
			fmt.Printf("ID:         #%d\nName:       %s\nOwner:      %s\nBlocked by: %s\nLabels:     %s\nCreated:    %s\nDone:       %s\n\n%s",
				t.ID, t.Name, cmp.Or(t.Owner, "-"), cmp.Or(blockers(t, openIDs(tasks)), "-"),
				cmp.Or(labelList(t.Labels, "\n            "), "-"), stamp(t.Created), done, t.Body)
			return nil
		},
	}
}

func doneCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "done ID...",
		Short:             "Mark tasks done",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return each(args, "done", func(dir string, id int) error {
				t, err := load(dir, id)
				if err != nil || !t.Done.IsZero() {
					return err
				}
				t.Done = time.Now()
				return save(dir, t)
			})
		},
	}
}

func deleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "delete ID...",
		Short:             "Delete tasks",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return each(args, "deleted", func(dir string, id int) error {
				if _, err := load(dir, id); err != nil {
					return err
				}
				return os.Truncate(path(dir, id), 0)
			})
		},
	}
}

func each(args []string, verb string, fn func(dir string, id int) error) error {
	dir, err := project()
	if err != nil {
		return err
	}
	failed := false
	for _, arg := range args {
		id, err := parseID(arg)
		if err == nil {
			err = fn(dir, id)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			failed = true
			continue
		}
		fmt.Printf("#%d %s\n", id, verb)
	}
	if failed {
		os.Exit(1)
	}
	return nil
}

func home() string {
	dir, _ := os.UserHomeDir()
	return dir
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func store() (string, error) {
	dir, err := os.UserConfigDir()
	return filepath.Join(dir, "task"), err
}

func project() (string, error) {
	// Checked in this order:
	// 1. Git first. Walk up to the nearest .git.
	//    - A .git folder: that folder's parent is the project.
	//    - A .git file (worktree or submodule): read its gitdir: path.
	//      - If that folder has a commondir file, it is a worktree. The project is the main repository,
	//        so every worktree shares one list, which is what parallel agents in worktrees need.
	//      - Otherwise it is a submodule, and the project is the folder holding the .git file.
	//    - A .git file with no gitdir: line is skipped, and the walk goes on.
	//    - Read directly from the files, so it needs no git binary and starts no process on every prompt.
	// 2. No git. Walk up to the nearest folder a task was ever created in, even if every task in it
	//    was deleted since. The walk never matches the home directory, so tasks kept in ~ don't take
	//    over every non-git folder under it. This covers an agent that cds into a subfolder: Claude
	//    Code only resets the shell when it leaves the session folder.
	// 3. Otherwise the current folder is the project. Its first task create makes it one.
	s, err := store()
	if err != nil {
		return "", err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, dir := range ancestors(cwd) {
		if root, ok := gitRoot(dir); ok {
			return projectDir(s, root), nil
		}
	}
	for _, dir := range ancestors(cwd) {
		if dir != home() && isDir(projectDir(s, dir)) {
			return projectDir(s, dir), nil
		}
	}
	return projectDir(s, cwd), nil
}

func projectDir(s, root string) string {
	name := url.QueryEscape(root)
	if len(name) > 255 {
		short := root
		for len(url.QueryEscape(short)) > 236 {
			_, size := utf8.DecodeLastRuneInString(short)
			short = short[:len(short)-size]
		}
		sum := sha256.Sum256([]byte(root))
		name = fmt.Sprintf("%s…%x", url.QueryEscape(short), sum[:8])
	}
	return filepath.Join(s, name)
}

func projects() ([]string, error) {
	s, err := store()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var dirs []string
	for _, e := range entries {
		dirs = append(dirs, filepath.Join(s, e.Name()))
	}
	return dirs, err
}

func ancestors(dir string) []string {
	dirs := []string{dir}
	for parent := filepath.Dir(dir); parent != dir; dir, parent = parent, filepath.Dir(parent) {
		dirs = append(dirs, parent)
	}
	return dirs
}

func gitRoot(dir string) (string, bool) {
	dotGit := filepath.Join(dir, ".git")
	if isDir(dotGit) {
		return dir, true
	}
	data, err := os.ReadFile(dotGit)
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
	if err != nil || !ok {
		return "", false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	common, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return dir, true
	}
	commonDir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	return filepath.Dir(commonDir), true
}

func find(arg string) (string, Task, error) {
	dir, err := project()
	if err != nil {
		return "", Task{}, err
	}
	id, err := parseID(arg)
	if err != nil {
		return "", Task{}, err
	}
	t, err := load(dir, id)
	return dir, t, err
}

func singleLine(what, s string) error {
	if strings.TrimSpace(s) == "" || strings.ContainsFunc(s, unicode.IsControl) {
		return fmt.Errorf("%s must be a single line of text", what)
	}
	return nil
}

func parseID(s string) (int, error) {
	id, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid ID %q: give a number like 12 or '#12'", s)
	}
	return id, nil
}

func pair(s, op string) (string, string, error) {
	k, v, ok := strings.Cut(s, op)
	if !ok || !validLabelPart(k) || !validLabelPart(v) {
		return "", "", fmt.Errorf("invalid label %q: write key%svalue, with no '=', ',' or whitespace in either", s, op)
	}
	return k, v, nil
}

func validLabelPart(s string) bool {
	return s != "" && !strings.ContainsAny(s, "=,") && !strings.ContainsFunc(s, unicode.IsSpace)
}

func link(url, text string) string {
	if info, err := os.Stdout.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return url
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func openIDs(tasks []Task) map[int]bool {
	open := map[int]bool{}
	for _, t := range tasks {
		open[t.ID] = t.Done.IsZero()
	}
	return open
}

func blockers(t Task, open map[int]bool) string {
	var ids []string
	for _, id := range t.BlockedBy {
		if open[id] {
			ids = append(ids, "#"+strconv.Itoa(id))
		}
	}
	return strings.Join(ids, ",")
}

func labelList(labels map[string]string, sep string) string {
	var pairs []string
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		pairs = append(pairs, k+"="+labels[k])
	}
	return strings.Join(pairs, sep)
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func stamp(t time.Time) string {
	return fmt.Sprintf("%s (%s ago)", t.Local().Format("2006-01-02 15:04"), ago(t))
}

func path(dir string, id int) string {
	return filepath.Join(dir, strconv.Itoa(id)+".json")
}

func load(dir string, id int) (Task, error) {
	t := Task{ID: id}
	data, err := os.ReadFile(path(dir, id))
	if err != nil || len(data) == 0 {
		return t, fmt.Errorf("#%d not found", id)
	}
	err = json.Unmarshal(data, &t)
	return t, err
}

func loadAll(dir string) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	name, _, cut := strings.Cut(filepath.Base(dir), "…")
	project, _ := url.QueryUnescape(name)
	if cut {
		project += "..."
	}
	var tasks []Task
	for _, e := range entries {
		id, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		if t, err := load(dir, id); err == nil {
			t.Project = project
			tasks = append(tasks, t)
		}
	}
	slices.SortFunc(tasks, func(a, b Task) int { return cmp.Compare(a.ID, b.ID) })
	return tasks, nil
}

func create(dir string, t *Task) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for t.ID = 1; ; t.ID++ {
		f, err := os.OpenFile(path(dir, t.ID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		f.Close()
		return save(dir, *t)
	}
}

func save(dir string, t Task) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d", path(dir, t.ID), os.Getpid())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path(dir, t.ID))
}

func current() []Task {
	dir, err := project()
	if err != nil {
		return nil
	}
	tasks, _ := loadAll(dir)
	return tasks
}

func completeID(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeIDs(cmd, args, toComplete)
}

func completeIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var ids []string
	for _, t := range current() {
		ids = append(ids, fmt.Sprintf("%d\t%s", t.ID, t.Name))
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

func completeLabels(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var pairs []string
	for _, t := range current() {
		for k, v := range t.Labels {
			pairs = append(pairs, k+"="+v)
		}
	}
	slices.Sort(pairs)
	return slices.Compact(pairs), cobra.ShellCompDirectiveNoFileComp
}
