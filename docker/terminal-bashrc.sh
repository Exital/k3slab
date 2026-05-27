# k3slab interactive terminal: colorized prompt (no hostname).
# Loaded via bash --rcfile for the web PTY, and sourced from /root/.bashrc as a fallback.

[[ -n "${__k3slab_prompt_loaded:-}" ]] && return 0
__k3slab_prompt_loaded=1

# Prompt colors (requires TERM=xterm-256color).
__k3slab_c_reset='\[\033[0m\]'
__k3slab_c_user='\[\033[1;32m\]'    # user
__k3slab_c_path='\[\033[1;34m\]'    # cwd
__k3slab_c_sym='\[\033[0;37m\]'     # # suffix (light gray)

PS1="${__k3slab_c_user}\u${__k3slab_c_reset}:${__k3slab_c_path}\w${__k3slab_c_reset}${__k3slab_c_sym}# ${__k3slab_c_reset}"
