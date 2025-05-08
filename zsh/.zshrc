export ZSH=~/.oh-my-zsh

ZSH_THEME="jacobmatthews"
plugins=(git)

source $ZSH/oh-my-zsh.sh

# Load NVM
export NVM_DIR=~/.nvm
[[ -s "$NVM_DIR/nvm.sh" ]] && source "$NVM_DIR/nvm.sh" --no-use
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"  # This loads nvm bash_completion

export PATH="$HOME/.nvm/versions/node/v23.6.0/bin:$PATH"
export PATH="/usr/local/go/bin:$PATH"
export PATH="$HOME/go/bin:$PATH"
export PATH="$HOME/.tfenv/bin:$PATH"
export GPG_TTY=$(tty)

alias vim=nvim

[ -s "$HOME/.secrets.sh" ] && source $HOME/.secrets.sh
