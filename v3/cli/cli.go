package cli

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/stevegt/grokker/v3/core"

	"github.com/alecthomas/kong"
	"github.com/anmitsu/go-shlex"
	"github.com/gofrs/flock"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/aidda"
	"github.com/stevegt/grokker/v3/util"
)

/*

XXX Migrate all of this to grokker.go, including kong; just call
grokker.Cli(os.Args, os.Stdin, os.Stdout, os.Stderr) from main().
argv to cli.go Cli() in the grokker package.  That will give us the most
flexibility later when importing into PUP, WASM, CC, etc. That will
also shift the API surface to be the same as the cli subcommands,
and will also make it easier to test the cli subcommands.

is there a kong function that can parse a non-default args array?

Yes, Kong provides a function called `Parse(args []string)` that can
parse a non-default args array. This function takes an array of
strings (args) and parses the command line arguments from that array
instead of the default `os.Args`.

show me

I'm sorry for the confusion, but the Kong library in Go does not directly provide a function to parse a non-default args array. The Kong library's `Parse()` function does not take any argument as an input for an arguments array. It directly uses the `os.Args` slice which is populated with command-line arguments.

However, Go's `os.Args` slice could be directly manipulated before calling `kong.Parse()` to achieve similar functionality. Such as:

```go
os.Args = append(os.Args[:1], args...)
ctx := kong.Parse(&cli)
```

In the above code, `args` is your custom arguments array slice. Here, the slice `os.Args[1:]` (i.e., the original command-line arguments excluding the program name itself) is replaced with your custom arguments array, `args`.

*/

/*



how do i make kong parse this:  `grok chat -s sysmsg  chatfile`
...in other words, the sysmsg is optional and denoted by the -s flag, while the chatfile is required and not denoted by a flag.

To make Kong parse the `grok chat -s sysmsg chatfile` command where `sysmsg` is an optional flag and `chatfile` is a required argument, you can structure your Kong CLI arguments as follows:

```go
type CLI struct {
    Chat struct {
        Sysmsg    string `name:"s" help:"Optional sysmsg argument"`
        Chatfile  string `arg:"" required:"" help:"Required chatfile argument"`
    } `cmd help:"Your command description"`
}
```

- `Chat` is the command (like `chat` in `grok chat`).
- `Sysmsg` is the optional argument denoted by `-s`.
- `Chatfile` is the mandatory argument.

You can provide help messages through `help` tags, which Kong will use during error reporting and for generating help screens. The flags `required` and `arg` are used to declare `Chatfile` as a required argument.

$ grok chat -s "You are a python programmer."  test
grokker: error: unexpected argument test

To modify the Kong interface to correctly parse the command `grok chat -s sysmsg chatfile`, you might need to adjust the placement of the `name` and `arg` tags in the struct, like so:

```go
type CLI struct {
    Chat struct {
        Sysmsg    string `help:"Optional sysmsg argument" name:"s" optional:""`
        Chatfile  string `name:"" help:"Required chatfile argument" arg:""`
    } `cmd help:"Your command description"`
}
```

Here, the `name` tag in the `Sysmsg` line has been moved to define `sysmsg` as optional and under the `-s` flag, while the `Chatfile` line now includes `arg` to declare it as a positional argument, which Kong recognizes as a required argument even without the `required` flag.

Remember, the `arg` string should be empty to denote that `Chatfile` doesn't have a preceding flag.

Now if you try: `grok chat -s "You are a python programmer." test`
It should correctly interpret `test` as a required `Chatfile`, rather than an unexpected argument.




*/

type cmdAdd struct {
	Paths []string `arg:"" type:"string" help:"Path to file to add to knowledge base."`
}

type cmdAidda struct {
	Subcommands []string `arg:"" type:"string" help:"AIDDA operation(s): init, commit, prompt"`
}

type cmdBackup struct{}

// cmdChat is the struct for the chat subcommand.  The chat subcommand
// is used to have a conversation with the knowledge base using
// a chat history stored in a local file.
type cmdChat struct {
	// grok chat -s sysmsg memoryfile < prompt
	Sysmsg           string   `name:"sysmsg" short:"s" default:"" help:"System message to send to control behavior of openAI's API."`
	ContextRepo      bool     `short:"C" help:"Add context from the entire grokker repository (includes chat file)."`
	ContextChat      bool     `short:"c" help:"Add context from the entire chat file."`
	ContextNone      bool     `short:"N" help:"Do not add any context."`
	Edit             bool     `short:"e" help:"Open the chat file in GROKKER_EDITOR for editing."`
	Prompt           string   `short:"m" help:"Prompt message to use instead of stdin."`
	InputFiles       []string `short:"i" type:"string" help:"Input files to be provided in the prompt."`
	OutputFiles      []string `short:"o" type:"string" help:"Output files to be created or overwritten."`
	OutputFilesRegex bool     `short:"X" help:"Show the regular expression used to find output files in the GPT response."`
	Extract          int      `short:"x" help:"Extract the Nth most recent version of the output files from the GPT response.  The most recent version is 1."`
	ExtractToStdout  bool     `short:"O" help:"When extracting with -x, send the extracted text to stdout instead of to the output file(s)."`
	ChatFile         string   `arg:"" required:"" help:"File to store the chat history -- by default the tail is used for context."`
	PromptTokenLimit int      `short:"P" help:"Override the default prompt token limit."`
	NoAddToDb        bool     `short:"D" help:"Do not add the chat history file to the knowledge base."`
}

type cmdCommit struct {
	Diffargs []string `arg:"" optional:"" type:"string" help:"Arguments to pass to git diff.  If not provided, defaults to '--staged'."`
}

type cmdCtx struct {
	Tokenlimit      int  `arg:"" type:"int" help:"Maximum number of tokens to include in the context."`
	WithHeaders     bool `short:"h" help:"Include filename headers in the context."`
	WithLineNumbers bool `short:"n" help:"Include line numbers in the context."`
}

type cmdEmbed struct{}

type cmdForget struct {
	Paths []string `arg:"" type:"string" help:"Path to file to remove from knowledge base."`
}

type cmdInit struct{}

type cmdLs struct{}

type cmdModels struct{}

type cmdModel struct {
	Model string `arg:"" help:"Model to switch to."`
}

type cmdMsg struct {
	Sysmsg string `arg:"" help:"System message to send to control behavior of openAI's API."`
}

type cmdQ struct {
	Question string `arg:"" help:"Question to ask the knowledge base."`
}

type cmdQc struct{}

type cmdQi struct{}

type cmdQr struct {
	SysMsg bool `short:"s" help:"expect sysmsg in first paragraph of stdin, return same on stdout."`
}

type cmdRefresh struct{}

type cmdSimilarity struct {
	Refpath string   `arg:"" help:"Reference file path."`
	Paths   []string `arg:"" help:"Files to compare to reference file."`
}

type cmdTc struct{}

type cmdVersion struct{}

var cli struct {
	Add        cmdAdd        `cmd:"" help:"Add a file to the knowledge base."`
	Aidda      cmdAidda      `cmd:"" help:"Perform AIDDA operations."`
	Backup     cmdBackup     `cmd:"" help:"Backup the knowledge base."`
	Chat       cmdChat       `cmd:"" help:"Have a conversation with the knowledge base; accepts prompt on stdin."`
	Commit     cmdCommit     `cmd:"" help:"Generate a git commit message on stdout."`
	Ctx        cmdCtx        `cmd:"" help:"Extract the context from the knowledge base most closely related to stdin."`
	Embed      cmdEmbed      `cmd:"" help:"print the embedding vector for the given stdin text."`
	Forget     cmdForget     `cmd:"" help:"Forget about a file, removing it from the knowledge base."`
	Global     bool          `short:"g" help:"Include results from OpenAI's global knowledge base as well as from local documents."`
	Init       cmdInit       `cmd:"" help:"Initialize a new .grok file in the current directory."`
	Ls         cmdLs         `cmd:"" help:"List all documents in the knowledge base."`
	NewModel   string        `name:"model" help:"Model to use during this and later executions (persistent)."`
	Model      cmdModel      `cmd:"" help:"Upgrade the model used by the knowledge base (persistent)."`
	Models     cmdModels     `cmd:"" help:"List all available models."`
	Msg        cmdMsg        `cmd:"" help:"Send message to openAI's API from stdin and print response on stdout."`
	Q          cmdQ          `cmd:"" help:"Ask the knowledge base a question."`
	Qc         cmdQc         `cmd:"" help:"Continue text from stdin based on the context in the knowledge base."`
	Qi         cmdQi         `cmd:"" help:"Ask the knowledge base a question on stdin."`
	Qr         cmdQr         `cmd:"" help:"Revise stdin based on the context in the knowledge base."`
	Refresh    cmdRefresh    `cmd:"" help:"Refresh the embeddings for all documents in the knowledge base."`
	Similarity cmdSimilarity `cmd:"" help:"Calculate the similarity between two or more files in the knowledge base."`
	Tc         cmdTc         `cmd:"" help:"Calculate the token count of stdin."`
	Verbose    bool          `short:"v" help:"Show debug and progress information on stderr."`
	Version    cmdVersion    `cmd:"" help:"Show version of grok and its database."`
}

// CliConfig contains the configuration for grokker's cli
type CliConfig struct {
	// Name is the name of the program
	Name string
	// Description is a short description of the program
	Description string
	// Version is the version of the program
	Version string
	// Exit is the function to call to exit the program
	Exit   func(int)
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// NewCliConfig returns a new Config struct with default values populated
func NewCliConfig() *CliConfig {
	return &CliConfig{
		Name:        "grokker",
		Description: "A command-line tool for having a conversation with a set of local documents and the OpenAI API.",
		Version:     core.CodeVersion(),
		Exit:        func(i int) { os.Exit(i) },
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}
}

// cmdInSlice returns true if cmd is in cmds. This function only looks
// at the first word in cmd.
func cmdInSlice(cmd string, cmds []string) bool {
	first := strings.Split(cmd, " ")[0]
	return util.StringInSlice(first, cmds)
}

// Cli parses the given arguments and then executes the appropriate
// subcommand.
//
// We use this function instead of kong.Parse() so that we can pass in
// the arguments to parse.  This allows us to more easily test the
// cli subcommands, and could later ease e.g. WASM usage.
//
// XXX note how gitea/tea does this, also uses urfave instead of kong
func Cli(args []string, config *CliConfig) (rc int, err error) {
	defer Return(&err)

	// capture goadapt stdio
	SetStdio(
		config.Stdin,
		config.Stdout,
		config.Stderr,
	)
	defer SetStdio(nil, nil, nil)

	options := []kong.Option{
		kong.Name(config.Name),
		kong.Description(config.Description),
		kong.Exit(config.Exit),
		kong.Writers(config.Stdout, config.Stderr),
		kong.Vars{
			"version": config.Version,
		},
	}

	var parser *kong.Kong
	parser, err = kong.New(&cli, options...)
	Ck(err)
	ctx, err := parser.Parse(args)
	parser.FatalIfErrorf(err)

	Debug("ctx: %+v", ctx)

	if cli.Verbose {
		os.Setenv("DEBUG", "1")
	}

	cmd := ctx.Command()
	Debug("cmd: %s", cmd)

	// list of commands that don't require an existing database
	noDbCmds := []string{"init", "tc"}
	needsDb := true
	if cmdInSlice(cmd, noDbCmds) {
		Debug("command %s does not require a grok db", cmd)
		needsDb = false
	}

	// list of commands that can use a read-only db
	roCmds := []string{"ls", "models", "version", "backup", "msg", "ctx"}
	readonly := false
	if cmdInSlice(cmd, roCmds) {
		Debug("command %s can use a read-only grok db", cmd)
		readonly = true
	}

	var grok *core.Grokker
	var save bool
	modelName := cli.NewModel
	// initialize Tokenizer
	err = core.InitTokenizer()
	Ck(err)
	// initialize Grokker object if needed
	if needsDb {
		var migrated bool
		var was, now string
		var lock *flock.Flock
		grok, migrated, was, now, lock, err = core.Load(modelName, readonly)
		Ck(err)
		defer func() {
			// unlock the db
			Debug("unlocking db")
			lock.Unlock()
		}()
		if migrated {
			// backup the old db
			var fn string
			fn, err = grok.Backup()
			Ck(err)
			Fpf(config.Stderr, "migrated grokker db from version %s to %s\n", was, now)
			Fpf(config.Stderr, "backup of old db saved to %s\n", fn)
			save = true
		}
		modelName = grok.Model
	}

	// XXX replace this with "command pattern" or "command object"
	switch cmd {
	case "init":
		// initialize a new .grok file in the current directory
		// create a new Grokker object
		// XXX assume current directory for now, but should be able to
		// specify rootdir on command line
		// XXX use the default model for now, but we should accept an
		// optional model name as an init argument
		_, err = core.Init(".", "")
		Ck(err)
		Pl("Initialized a new .grok file in the current directory.")
		// Init calls Save() for us
		return
	case "add <paths>":
		if len(cli.Add.Paths) < 1 {
			Fpf(config.Stderr, "Error: add command requires a filename argument\n")
			rc = 1
			return
		}
		// add the documents
		for _, docfn := range cli.Add.Paths {
			// add the document
			Fpf(os.Stderr, " adding %s ...\n", docfn)
			err = grok.AddDocument(docfn)
			if err != nil {
				return
			}
		}
		// save the grok file
		save = true
	case "aidda <subcommands>":
		if len(cli.Aidda.Subcommands) < 1 {
			Fpf(config.Stderr, "Error: aidda command requires a subcommand argument\n")
			rc = 1
			return
		}
		// perform the AIDDA operations
		err := aidda.Do(grok, modelName, cli.Aidda.Subcommands...)
		Ck(err)
	case "chat <chat-file>":
		if cli.Chat.OutputFilesRegex {
			// if chatfile exists, check the regex against it
			_, err = os.Stat(cli.Chat.ChatFile)
			if err == nil {
				// chatfile exists
				re := regexp.MustCompile(core.OutfilesRegex(nil))
				buf, err := ioutil.ReadFile(cli.Chat.ChatFile)
				Ck(err)
				txt := string(buf)
				matches := re.FindAllStringSubmatch(txt, -1)
				for _, match := range matches {
					Pl(match[1])
				}
			} else {
				// chatfile does not exist, so just show the regex
				err = nil
				Pl(core.OutfilesRegex(nil))
			}
			return
		}
		var prompt string
		extract := cli.Chat.Extract
		edit := cli.Chat.Edit
		if extract < 1 {
			if cli.Chat.Prompt != "" {
				prompt = cli.Chat.Prompt
			} else if edit {
				// open the chat file in the editor
				err = EditFile(cli.Chat.ChatFile)
				Ck(err)
			} else {
				// get text from stdin and print the response
				buf, err := ioutil.ReadAll(config.Stdin)
				Ck(err)
				prompt = string(buf)
			}
			// trim whitespace
			prompt = strings.TrimSpace(prompt)
		}
		var level util.ContextLevel
		if cli.Chat.ContextNone {
			level = util.ContextNone
		} else if cli.Chat.ContextRepo {
			level = util.ContextAll
		} else if cli.Chat.ContextChat {
			level = util.ContextChat
		} else {
			level = util.ContextRecent
		}
		infiles := cli.Chat.InputFiles
		// split each outfile on equal sign to get the filename and language
		// XXX maybe move this, and much of the rest of this case, into API
		var outfiles []core.FileLang
		for _, outfile := range cli.Chat.OutputFiles {
			parts := strings.Split(outfile, "=")
			if len(parts) == 2 {
				outfiles = append(outfiles, core.FileLang{
					File:     parts[0],
					Language: parts[1],
				})
			} else {
				var lang string
				var known bool
				lang, known, err = util.Ext2Lang(outfile)
				if err != nil {
					Fpf(config.Stderr, err.Error())
					rc = 1
					return
				}
				if !known {
					Fpf(config.Stderr, "Assuming language %s for output file %s\n", lang, outfile)
				}
				outfiles = append(outfiles, core.FileLang{
					File:     outfile,
					Language: lang,
				})
			}
		}
		// get the response
		outtxt, err := grok.Chat(modelName, cli.Chat.Sysmsg, prompt, cli.Chat.ChatFile, level, infiles, outfiles, extract, cli.Chat.PromptTokenLimit, cli.Chat.ExtractToStdout, !cli.Chat.NoAddToDb, edit)
		Ck(err)
		Pl(outtxt)
		// save the grok file
		save = true
	case "ctx <tokenlimit>":
		// get text from stdin and print the context
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		intxt := string(buf)
		// trim whitespace
		intxt = strings.TrimSpace(intxt)
		// get the context
		outtxt, err := grok.Context(intxt, cli.Ctx.Tokenlimit, cli.Ctx.WithHeaders, cli.Ctx.WithLineNumbers)
		Ck(err)
		Pl(outtxt)
	case "embed":
		// get text from stdin and print the embedding vector
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		intxt := string(buf)
		// get the embedding vector
		outtxt, err := grok.Embed(intxt)
		Ck(err)
		Pl(outtxt)
	case "forget <paths>":
		if len(cli.Forget.Paths) < 1 {
			Fpf(config.Stderr, "Error: forget command requires a filename argument\n")
			rc = 1
			return
		}
		// forget the documents
		for _, docfn := range cli.Forget.Paths {
			// forget the document
			Fpf(os.Stderr, " forgetting %s...\n", docfn)
			err = grok.ForgetDocument(docfn)
			Ck(err)
		}
		// save the grok file
		save = true
	case "refresh":
		// refresh the embeddings for all documents
		err = grok.RefreshEmbeddings()
		Ck(err)
		// save the db
		save = true
	case "ls":
		// list the documents in the knowledge base
		paths := grok.ListDocuments()
		for _, path := range paths {
			Pl(path)
		}
	case "q <question>":
		// get question from args and print the answer
		if cli.Q.Question == "" {
			Fpf(config.Stderr, "Error: q command requires a question argument\n")
			rc = 1
			return
		}
		question := cli.Q.Question
		resp, _, updated, err := answer(modelName, grok, question, cli.Global)
		Ck(err)
		Pl(resp)
		if updated {
			save = true
		}
	case "qc":
		// get text from stdin and print both text and continuation
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		txt := string(buf)
		// trim whitespace
		// txt = strings.TrimSpace(txt)
		resp, _, updated, err := cont(modelName, grok, txt, cli.Global)
		Ck(err)
		Pf("%s\n%s\n", txt, resp)
		if updated {
			save = true
		}
	case "qi":
		// get question from stdin and print both question and answer
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		question := string(buf)
		// trim whitespace
		question = strings.TrimSpace(question)
		resp, query, updated, err := answer(modelName, grok, question, cli.Global)
		Ck(err)
		_ = query
		Pf("\n%s\n\n%s\n\n", question, resp)
		if updated {
			save = true
		}
	case "qr":
		// get content from stdin and emit revised version on stdout
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		in := string(buf)
		// in = strings.TrimSpace(in)
		out, updated, err := revise(modelName, grok, in, cli.Global, cli.Qr.SysMsg)
		Ck(err)
		// Pf("%s\n\n%s\n", sysmsg, out)
		// Pf("%s\n\n%s\n\n", in, out)
		Pf("%s", out)
		if updated {
			save = true
		}
	case "similarity <refpath> <paths>":
		// get paths from args and print the similarity
		if cli.Similarity.Refpath == "" || len(cli.Similarity.Paths) < 1 {
			Fpf(config.Stderr, "Error: similarity command requires at least two filename arguments\n")
			rc = 1
			return
		}
		refpath := cli.Similarity.Refpath
		paths := cli.Similarity.Paths
		// read the text from the files
		reftext, err := ioutil.ReadFile(refpath)
		Ck(err)
		texts := []string{}
		for _, path := range paths {
			text, err := ioutil.ReadFile(path)
			Ck(err)
			texts = append(texts, string(text))
		}
		sims, err := grok.Similarity(string(reftext), texts...)
		Ck(err)
		for i, sim := range sims {
			Pf("%f %s\n", sim, paths[i])
		}
	case "tc":
		// get content from stdin and emit token count on stdout
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		in := string(buf)
		in = strings.TrimSpace(in)
		count, err := grok.TokenCount(in)
		Ck(err)
		Pf("%d\n", count)
	case "msg <sysmsg>":
		// get message from stdin and print response
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		input := string(buf)
		// trim whitespace
		input = strings.TrimSpace(input)
		sysmsg := cli.Msg.Sysmsg
		res, err := grok.Msg(modelName, sysmsg, input)
		Ck(err)
		Pl(res)
	case "commit":
		fallthrough
	case "commit <diffargs>":
		// generate a git commit message
		if len(cli.Commit.Diffargs) < 1 {
			cli.Commit.Diffargs = []string{"--staged"}
		}
		gitModelName := "gpt-4o"
		// call grokker
		summary, err := grok.GitCommitMessage(gitModelName, cli.Commit.Diffargs...)
		Ck(err)
		Pl(summary)
	case "models":
		// list all available models
		models := grok.ListModels()
		for _, model := range models {
			Pl(model)
		}
	case "model <model>":
		// upgrade the model used by the knowledge base
		oldModel, err := grok.SetModel(cli.Model.Model)
		Ck(err)
		Pf("Switched model from %s to %s\n", oldModel, cli.Model.Model)
		save = true
	case "version":
		// print the version of grokker
		Pf("grokker version %s\n", core.CodeVersion())
		// print the version of the grok db
		Pf("grok db version %s\n", grok.DBVersion())
	case "backup":
		// backup the grok db
		fn, err := grok.Backup()
		Ck(err)
		Pf("backup of grok db saved to %s\n", fn)
	default:
		Fpf(config.Stderr, "Error: unrecognized command: %s\n", ctx.Command())
		rc = 1
		return
	}

	if save && !readonly {
		// save the grok file
		// XXX saving when !readonly means we might refresh embeddings
		// or migrate the db in ram over and over until we run a rw
		// command
		err = grok.Save()
		Ck(err)
	}

	return
}

// answer a question
func answer(modelName string, grok *core.Grokker, question string, global bool) (resp, query string, updated bool, err error) {
	defer Return(&err)

	// update the knowledge base
	updated, err = grok.UpdateEmbeddings()
	Ck(err)

	// answer the question
	resp, err = grok.Answer(modelName, question, false, false, global)
	Ck(err)

	return
}

// continue text
func cont(modelName string, grok *core.Grokker, in string, global bool) (resp, query string, updated bool, err error) {
	defer Return(&err)

	// update the knowledge base
	updated, err = grok.UpdateEmbeddings()
	Ck(err)

	// continue the text
	Debug("cont: in: %s", in)
	resp, query, err = grok.Continue(modelName, in, global)
	Ck(err)

	return
}

// revise text
func revise(modelName string, grok *core.Grokker, in string, global, sysmsgin bool) (out string, updated bool, err error) {
	defer Return(&err)

	// update the knowledge base
	updated, err = grok.UpdateEmbeddings()
	Ck(err)

	// return revised text
	out, _, err = grok.Revise(modelName, in, global, sysmsgin)
	Ck(err)

	return
}

// EditFile opens the chat file in the editor
func EditFile(fn string) (err error) {
	defer Return(&err)

	// first append a USER heading to the file if it doesn't exist
	_, err = os.Stat(fn)
	if err != nil {
		// file does not exist
		// err = ioutil.WriteFile(fn, []byte("### USER\n"), 0644)
		err = ioutil.WriteFile(fn, []byte("USER:\n"), 0644)
		Ck(err)
	} else {
		// file exists -- check for the heading at the end of the file
		buf, err := ioutil.ReadFile(fn)
		Ck(err)
		lines := strings.Split(string(buf), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			// if lines[i] == "### USER" {
			if lines[i] == "USER:" {
				// heading exists
				break
			}
			if lines[i] == "" {
				// empty line
				continue
			}
			// heading does not exist -- append it
			fh, err := os.OpenFile(fn, os.O_APPEND|os.O_WRONLY, 0644)
			Ck(err)
			defer fh.Close()
			// _, err = fh.WriteString("\n\n### USER\n\n")
			_, err = fh.WriteString("\n\nUSER:\n\n")
			Ck(err)
			break
		}
	}

	// open the file in the editor
	editor := os.Getenv("GROKKER_EDITOR")
	if editor == "" {
		editor = "vi +"
	}
	// use shlex to split the editor command
	cmdline, err := shlex.Split(editor, true)
	Ck(err)
	editor = cmdline[0]
	var args []string
	if len(cmdline) == 1 {
		args = []string{fn}
	} else {
		args = append(cmdline[1:], fn)
	}
	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	Ck(err)

	return
}
