In neovim, get the diffview plugin:

```bash
mkdir -p ~/.vim/bundle
cd ~/.vim/bundle
git clone git@github.com:sindrets/diffview.nvim.git
```

Add something like the following to your `init.vim` or
.vimrc file:

```vim
" diffview hotkeys
map <leader>dv :DiffviewOpen<CR>
map <leader>dc :DiffviewClose<CR>
map <leader>dl :DiffviewLog<CR>
map <leader>dh :DiffviewFileHistory<CR>

" aidda3 hotkeys
" recognize .aidda3/prompt as filetype aidda3_prompt
au BufNewFile,BufRead prompt setfiletype aidda3_prompt
" set a hotkey to easily run the grok aidda commit prompt command
map <leader>o :!grok aidda commit<CR>
" see ~/.vim/after/ftplugin/aidda3_prompt.vim for <leader>v mapping
```

Add the following to your `~/.vim/after/ftplugin/aidda3_prompt.vim` file:

```vim
nmap <buffer> <leader>v :!grok aidda commit prompt<CR>
nmap <buffer> <leader>t :!grok aidda test<CR>
```

What the above does:

- `<leader>dv` opens the diffview plugin
- `<leader>dc` closes the diffview plugin
- `<leader>dl` opens the git log using the diffview plugin
- `<leader>dh` opens the git file history using the diffview plugin
- `<leader>o` runs the grok aidda commit command
- `<leader>v` runs the grok aidda commit prompt command

Typical workflow: 

`<leader>` is by default `\` in neovim, but you can change it to
something else if you like -- see `:help mapleader` for more info.

- edit .aidda/prompt, providing input and output filenames and a
  GPT prompt
- press `<leader>v` to run the grok aidda commit prompt command, which
  does a `git add -A` and a `git commit` with an auto-generated commit
  message, then reads the prompt and input files, sends it all to
  OpenAI's API, and writes the output to the output file(s)
- press `<leader>dv` to see/edit the diff of the changes made by aidda
- press `<leader>dc` to close the diff 
- optionally press `<leader>dl` to see the git log
- optionally press `<leader>dh` to see the git file history
- optionally press `<leader>t` to run the grok aidda test command --
  this will run `go test -v` and put the test results in .aidda/test,
  which wil then be sent to OpenAI's API as part of the next prompt
  message
- alter the prompt file as needed and repeat the process



