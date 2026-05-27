# Load k3slab terminal prompt for interactive shells.
# shellcheck shell=bash
if [[ $- == *i* ]] && [[ -f /usr/local/lib/k3slab/terminal-bashrc.sh ]]; then
  # shellcheck source=/dev/null
  . /usr/local/lib/k3slab/terminal-bashrc.sh
fi
