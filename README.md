# grokker

A tool for interactive conversation with one or more documents -- for
research, training, and rapid learning.  Uses OpenAI API services for
backend -- see disclaimer below.

Grokker helped create this document in a VIM session by reading its
own source code along with iterations of this document.

## Example

```

$ grok init
Initialized a new .grok file in the current directory.

$ grok add README.md cmd/grok/main.go grokker.go TODO.md

Creating .grok file... adding README.md... adding cmd/grok/main.go...
adding grokker.go... adding TODO.md... done!

$ grok q "What is grokker?"

Grokker is a tool for interactive conversation with one or more
documents, used for research, training, and rapid learning. It
utilizes OpenAI API services for the backend. Essentially, you can
input documents into Grokker and it will use natural language
processing to analyze the text and help you answer questions about
those documents.

$ grok q "Describe grokker's subcommands."

Grokker has several subcommands, including:

1. `init`: Initializes a new .grok file in the current directory.
2. `add`: Adds one or more documents to the database.
3. `refresh`: Refreshes the embeddings for all documents.
4. `ls`: Lists all the documents in the .grok file.
5. `q`: Asks a question and returns an answer. You can include the
   `-g` flag to also search the OpenAI global knowledge base.
6. `qi`: Reads a question from standard input and returns an answer.

The `init` subcommand is only used once to create the grok file, while
the rest are used to manage and interact with the documents in the
grok file.

$ grok q "What is the `qi` subcommand for?"

The 'qi' subcommand allows you to ask a question by typing it in to
the standard input rather than passing it as a command-line argument.
For example, you could say:

  echo "What is the 'qi' subcommand for?" | grok qi 

This would read the question from standard input and return the
answer.  This subcommand is especially useful in editor sessions --
more about this below. ```

## Installation

``` go install github.com/stevegt/grokker/cmd/grok ``` You'll need an
account with OpenAI and an API key. You can create and manage your API
keys in your OpenAI dashboard.

Before using Grokker, you'll need to set your API key as an
environment variable in your terminal session or .bashrc with the
following command:

``` export OPENAI_API_KEY=<your_api_key> ```

## Tell me more about the `qi` subcommand

The `qi` subcommand allows you to ask a question by typing it in to
the standard input rather than passing it as a command-line argument.
The question is read from stdin and the answer is generated using the
Grokker library. It's a way to generate quick answers to questions
without having to provide the question as a command-line argument.

The `qi` subcommand enables you to use grokker as a chat client in an
editor session by typing questions directly in your document and
receiving answers inline after the question.   

## Using grokker as a chat client in an editor session

Using Grokker as a chat client in an editor session can help you
quickly find and summarize information from a set of documents in a
local directory tree, including the documents you are editing. This
can make your development, research, or learning process more
efficient and streamlined. Additionally, having the context of your
editor session available as part of the chat history can help you
better keep track of and synthesize information as you work.

It's a quick way to build a document and was used to build this one.

Using grokker as a chat client in an editor session is also a way to
access the backend servers used by ChatGPT without being constrained
by the ChatGPT web frontend, all while maintaining your own chat
history and any additional context in your own local files,
versionable in git.  

### How can I use grokker in a VIM editor session?

To use the `qi` subcommand in a VIM editor session, you can add a
keyboard mapping to your vimrc file. Here's an example mapping:

``` :map <leader>g vap:!grok qi<CR> ```

This mapping will allow you to ask a question by typing it in VIM and
then pressing `<leader>g`. The question will be sent as input to the
`qi` subcommand of Grokker, and the answer will be inserted into the
buffer. Note that the mapping assumes that Grokker's `grok` command is
installed and in your system path.

Here is a more complete mapping that first saves the current file so
its most recent content will be included in the query context.  This
mapping uses a slightly different method to select the current
paragraph to be used in the query:

``` :map <leader>g :w<CR>{V}:!grok qi<CR>gqG ```

Experiment with variations on these mappings -- you might emphasize
more recent context by including the previous two paragraphs as part
of the query, or the most recent 50 lines, or the output of `git
diff`, etc.  (Future versions of grokker are likely to help with this
by timestamping individual document chunks and prioritizing more
recent edits, but that's likely to wait for the key-value db rewrite
mentioned below.)

## Tell me more about the `-g` flag

The `-g` flag is an optional parameter that you can include when
running the `q` subcommand. It stands for "global" and when included,
Grokker will provide answers not only from the local documents that
you've added but also from OpenAI's global knowledge base. This means
that you'll get a wider range of potentially useful answers, but it
may take longer to receive your results as the global knowledge base
is larger and may take more time to search through. If you don't
include the `-g` flag, Grokker will prefer the local documents that
you've added.

## What are the `models` and `upgrade` subcommands?

The `models` subcommand is used to list all the available OpenAI
models for text processing in Grokker, including their name and
maximum token limit. 

The `upgrade` subcommand is used to upgrade the local .grok db to a
newer model. It only allows upgrading the model to a model with a
larger token limit. (Downgrading would require re-chunking and
re-embedding all documents in the db to convert them to the smaller
chunk size.)

## Does the `grok` command conflict with any other common UNIX/Linux commands?

Yes.  For instance, Jordan Sissel's log file analysis tool also uses a
`grok` command.  If you want to install grokker on the same machine,
you can install it using an alternate command name.  Here's an example
of installing grokker as `grokker` instead of `grok`:

``` 
cd /tmp 
git clone http://github.com/stevegt/grokker 
cd grokker/cmd/grok/ 
go build -o grokker 
cp grokker $GOPATH/bin 
```

## Is grokker done?  What are some of the outstanding items?

Some outstanding items mentioned in the code and TODO.md are:

- replace JSON storage with kv (key-value) storage
- add 'sh' subcommand to support dot commands for directives such as
  adding a document, updating all embeddings, and reviewing a file for
  recently added context, acting otherwise as a chat client and adding
  each message to context, and adding each tx/rx message to context
  tree
- add 'vet' subcommand to look for inconsistencies, possibly by
  comparing summaries with individual chunks, or by negating and
  comparison; see demorgan, etc.

# Important disclaimer regarding sensitive and confidential
information

Using OpenAI's API services to analyze documents means that any
document you `grok add`, and any question you ask of grokker, will be
broken into chunks and sent to OpenAI's servers twice -- first to
generate context embedding vectors, and again as relevant context when
you run `grok q` or `grok qi`.

If any of your document's content is sensitive or confidential, you'll
want to review OpenAI's policies regarding data usage and retention.

Additionally, some topics are banned by OpenAI's policies, so be sure
to review and comply with their guidelines in order to prevent your
API access being suspended or revoked. 

As always, it's a good idea to review the terms and conditions of any
API service you're considering using to ensure that you're comfortable
with how your data will be handled.


