export ZSH=~/.oh-my-zsh

ZSH_THEME="jacobmatthews"
plugins=(git)

source $ZSH/oh-my-zsh.sh

# fnm
FNM_PATH="/opt/homebrew/opt/fnm/bin"
if [ -d "$FNM_PATH" ]; then
  eval "`fnm env`"
fi

export PATH="/usr/local/go/bin:$PATH"
export PATH="$HOME/go/bin:$PATH"
export PATH="$PATH:$HOME/.config/scripts/"
export PATH="$HOME/.local/bin:$PATH"
export GPG_TTY=$(tty)

alias vim=nvim

bindkey -s ^f "tmux-sessionizer\n"

[ -s "$HOME/.secrets.sh" ] && source $HOME/.secrets.sh

