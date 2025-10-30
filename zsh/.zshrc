export ZSH=~/.oh-my-zsh

ZSH_THEME="jacobmatthews"
plugins=(git)

source $ZSH/oh-my-zsh.sh

export PATH="/usr/local/go/bin:$PATH"
export PATH="$HOME/go/bin:$PATH"
export PATH="$PATH:$HOME/.config/scripts/"
export PATH="$HOME/.local/bin:$PATH"
eval "$(fnm env --use-on-cd --shell zsh)"

export GPG_TTY=$(tty)

alias vim=nvim

bindkey -s ^f "tmux-sessionizer\n"

[ -s "$HOME/.secrets.sh" ] && source $HOME/.secrets.sh

