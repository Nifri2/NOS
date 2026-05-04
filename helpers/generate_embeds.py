#!/usr/bin/env python3
"""
Generates Go embed directives and loading code in main.go based on anims.yaml.
Only processes animations with embed: true.
"""

import os
import re
import sys
import yaml

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ANIMS_YAML = os.path.join(PROJECT_ROOT, "anims.yaml")
MAIN_GO = os.path.join(PROJECT_ROOT, "main.go")

# Markers in main.go
EMBED_START = "// EMBED_START"
EMBED_END = "// EMBED_END"
LOAD_START = "// LOAD_START"
LOAD_END = "// LOAD_END"
APPEND_START = "// APPEND_START"
APPEND_END = "// APPEND_END"


def to_camel_case(name: str) -> str:
    """Convert snake_case to camelCase: eye_blink -> eyeBlink"""
    parts = name.split("_")
    return parts[0] + "".join(word.capitalize() for word in parts[1:])


def get_frame_consts(anim_type: str) -> tuple[str, str]:
    """Get the frame width/height constants for an animation type."""
    if anim_type == "eye":
        return "cmd.EyeFrameWidth", "cmd.EyeFrameHeight"
    elif anim_type == "mouth":
        return "cmd.MouthFrameWidth", "cmd.MouthFrameHeight"
    else:
        # Default to eye
        return "cmd.EyeFrameWidth", "cmd.EyeFrameHeight"


def generate_embed_block(animations: list) -> str:
    """Generate //go:embed directives and var declarations."""
    lines = [EMBED_START]
    for anim in animations:
        name = anim["name"]
        camel = to_camel_case(name)
        lines.append(f"//go:embed animations/{name}.animbyte")
        lines.append(f"var {camel}Data []byte")
        lines.append("")
    lines.append(EMBED_END)
    return "\n".join(lines)


def generate_load_block(animations: list) -> str:
    """Generate animation loading code."""
    lines = [LOAD_START]
    for anim in animations:
        name = anim["name"]
        anim_type = anim.get("type", "eye")
        camel = to_camel_case(name)
        width, height = get_frame_consts(anim_type)
        lines.append(f'\t{camel}Anim, err := cmd.LoadAnimation({camel}Data, {width}, {height}, "{name}")')
        lines.append("\tif err != nil {")
        lines.append(f'\t\tfmt.Println("Error loading {name}:", err)')
        lines.append("\t}")
        lines.append("")
    lines.append(f"\t{LOAD_END}")
    return "\n".join(lines)


def generate_append_block(animations: list) -> str:
    """Generate LoadedAnimations append code."""
    lines = [APPEND_START]
    lines.append("\tcmd.LoadedAnimations = nil")
    for anim in animations:
        name = anim["name"]
        camel = to_camel_case(name)
        lines.append(f"\tif {camel}Anim != nil {{")
        lines.append(f"\t\tcmd.LoadedAnimations = append(cmd.LoadedAnimations, {camel}Anim)")
        lines.append("\t}")
    lines.append(f"\t{APPEND_END}")
    return "\n".join(lines)


def replace_between_markers(content: str, start_marker: str, end_marker: str, replacement: str) -> str:
    """Replace content between markers (inclusive)."""
    pattern = re.compile(
        rf"^[ \t]*{re.escape(start_marker)}.*?^[ \t]*{re.escape(end_marker)}",
        re.MULTILINE | re.DOTALL
    )
    return pattern.sub(replacement, content)


def main():
    print(f"Loading configuration from {ANIMS_YAML}")

    if not os.path.exists(ANIMS_YAML):
        print(f"ERROR: {ANIMS_YAML} not found")
        sys.exit(1)

    with open(ANIMS_YAML, "r") as f:
        config = yaml.safe_load(f)

    # Filter to only animations with embed: true
    animations = [a for a in config.get("animations", []) if a.get("embed", False)]

    if not animations:
        print("No animations with embed: true found")
        sys.exit(0)

    print(f"Found {len(animations)} animation(s) to embed:")
    for a in animations:
        print(f"  - {a['name']} (type: {a.get('type', 'eye')})")

    # Read main.go
    if not os.path.exists(MAIN_GO):
        print(f"ERROR: {MAIN_GO} not found")
        sys.exit(1)

    with open(MAIN_GO, "r") as f:
        content = f.read()

    # Check for markers
    if EMBED_START not in content:
        print(f"\nERROR: Markers not found in main.go")
        print(f"Please add the following markers to main.go:")
        print(f"  {EMBED_START} / {EMBED_END} - around embed directives")
        print(f"  {LOAD_START} / {LOAD_END} - around LoadAnimation calls")
        print(f"  {APPEND_START} / {APPEND_END} - around LoadedAnimations appends")
        sys.exit(1)

    # Generate new blocks
    embed_block = generate_embed_block(animations)
    load_block = generate_load_block(animations)
    append_block = generate_append_block(animations)

    # Replace blocks
    content = replace_between_markers(content, EMBED_START, EMBED_END, embed_block)
    content = replace_between_markers(content, LOAD_START, LOAD_END, load_block)
    content = replace_between_markers(content, APPEND_START, APPEND_END, append_block)

    # Write back
    with open(MAIN_GO, "w") as f:
        f.write(content)

    print(f"\nUpdated {MAIN_GO}")
    print("Done!")


if __name__ == "__main__":
    main()
