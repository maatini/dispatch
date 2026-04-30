set -e

if [ -z "$__DEVBOX_SKIP_INIT_HOOK_8bc973428959154bcb236c8e0012a0a79c7ed0a7bdb24fcfd905d14e6cb8b5f7" ]; then
    . "/Volumes/SSD2TB/work/antigravity/dispatch/.devbox/gen/scripts/.hooks.sh"
fi

go build ./...
