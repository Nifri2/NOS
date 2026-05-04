# NOS

**N**itrous **O**xide / **N**ibble**OS** - Firmware for Nibble the protogen.

## Requirements

- [TinyGo](https://tinygo.org/) - Go compiler for embedded systems
- [Task](https://taskfile.dev/) - Task runner
- Python 3 with `pyyaml` and `rich`

## Tasks

Run tasks with `task <task-name>`. Use `task --list` to see all available tasks.

### Firmware

| Task | Description |
|------|-------------|
| `build:dispatcher` | Build dispatcher firmware |
| `build:worker-0` | Build worker-0 firmware |
| `build:worker-1` | Build worker-1 firmware |
| `build:worker-2` | Build worker-2 firmware |
| `build:worker-3` | Build worker-3 firmware |
| `build:all` | Build all firmwares |
| `flash:dispatcher` | Flash dispatcher firmware (with monitor) |
| `flash:worker-0` | Flash worker-0 firmware (with monitor) |
| `flash:worker-1` | Flash worker-1 firmware (with monitor) |
| `flash:worker-2` | Flash worker-2 firmware (with monitor) |
| `flash:worker-3` | Flash worker-3 firmware (with monitor) |
| `monitor` | Attach serial monitor to connected Pico |

### Animations

| Task | Description |
|------|-------------|
| `compile_anims` | Compile animations from `anims.yaml` to `.animbyte` files |
| `generate_embeds` | Update `main.go` with embed directives from `anims.yaml` |
| `update_anims` | Run both: compile animations and update embeds |

## Animation Pipeline

Animations are configured in `anims.yaml` and compiled to binary format for embedding in the firmware.

### Configuration (`anims.yaml`)

```yaml
animations:
  - name: eye_blink        # Output filename (eye_blink.animbyte)
    source: eye_blink.anim # Source folder in compiler/animations/
    type: eye              # Frame dimensions: "eye" or "mouth"
    embed: true            # Include in firmware
    rotation: 0            # 0, 90, 180, or 270 degrees
    matrix_width: 16       # LED matrix width
    matrix_height: 16      # LED matrix height
    brightness: 0.2        # 0.0 to 1.0
```

### Multiple outputs from one source

The same source animation can be compiled multiple times with different settings:

```yaml
  - name: mouth_idle_left
    source: mouth_idle.anim
    type: mouth
    embed: true
    rotation: 0
    matrix_width: 32
    matrix_height: 16
    brightness: 0.2

  - name: mouth_idle_right
    source: mouth_idle.anim
    type: mouth
    embed: true
    rotation: 180          # Rotated for the other side
    matrix_width: 32
    matrix_height: 16
    brightness: 0.2
```

### Workflow

1. Edit `anims.yaml` to add or modify animations
2. Run `task update_anims`
3. Build and flash firmware

### How it works

1. **`compile_anims`** - Runs `helpers/compile_all.py`:
   - Reads `anims.yaml`
   - Compiles each animation using `compiler/compiler/process.py`
   - Outputs `.animbyte` files to `animations/`

2. **`generate_embeds`** - Runs `helpers/generate_embeds.py`:
   - Reads `anims.yaml` (only entries with `embed: true`)
   - Updates `main.go` between marker comments:
     - `// EMBED_START` / `// EMBED_END` - embed directives
     - `// LOAD_START` / `// LOAD_END` - LoadAnimation calls
     - `// APPEND_START` / `// APPEND_END` - LoadedAnimations array

### Adding new animations

1. Create animation frames in `compiler/animations/<name>.anim/`
2. Add entry to `anims.yaml`
3. Run `task update_anims`
