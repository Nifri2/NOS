#!/usr/bin/env python3
"""
Generates Go code from anims.yaml:
- Embed directives and loading code in main.go
- AnimationID constants and side mapping in cmd/structs.go
"""

import os
import re
import sys
import yaml

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ANIMS_YAML = os.path.join(PROJECT_ROOT, "anims.yaml")
MAIN_GO = os.path.join(PROJECT_ROOT, "main.go")
STRUCTS_GO = os.path.join(PROJECT_ROOT, "cmd", "structs.go")

# Markers in main.go
EMBED_START = "// EMBED_START"
EMBED_END = "// EMBED_END"
LOAD_START = "// LOAD_START"
LOAD_END = "// LOAD_END"
APPEND_START = "// APPEND_START"
APPEND_END = "// APPEND_END"

# Markers in structs.go
ANIMID_START = "// ANIMID_START"
ANIMID_END = "// ANIMID_END"
MAPPING_START = "// MAPPING_START"
MAPPING_END = "// MAPPING_END"


def to_camel_case(name: str) -> str:
    """Convert snake_case to camelCase: eye_blink -> eyeBlink"""
    parts = name.split("_")
    return parts[0] + "".join(word.capitalize() for word in parts[1:])


def to_pascal_case(name: str) -> str:
    """Convert snake_case to PascalCase: eye_blink -> EyeBlink"""
    return "".join(word.capitalize() for word in name.split("_"))


def get_frame_consts(anim_type: str) -> tuple[str, str]:
    """Get the frame width/height constants for an animation type."""
    if anim_type == "eye":
        return "cmd.EyeFrameWidth", "cmd.EyeFrameHeight"
    elif anim_type == "mouth":
        return "cmd.MouthFrameWidth", "cmd.MouthFrameHeight"
    else:
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
    """Generate LoadedAnimations append code (sorted by ID for correct indexing)."""
    sorted_anims = sorted(animations, key=lambda a: a.get("id", 0))

    lines = [APPEND_START]
    lines.append("\tcmd.LoadedAnimations = nil")
    for anim in sorted_anims:
        name = anim["name"]
        camel = to_camel_case(name)
        lines.append(f"\tif {camel}Anim != nil {{")
        lines.append(f"\t\tcmd.LoadedAnimations = append(cmd.LoadedAnimations, {camel}Anim)")
        lines.append("\t}")
    lines.append(f"\t{APPEND_END}")
    return "\n".join(lines)


def generate_animid_block(all_animations: list) -> str:
    """Generate AnimationID constants for ALL animations (embedded + logical)."""
    sorted_anims = sorted(all_animations, key=lambda a: a.get("id", 0))

    lines = [ANIMID_START]
    lines.append("const (")

    for anim in sorted_anims:
        name = anim["name"]
        anim_id = anim.get("id", 0)
        pascal = to_pascal_case(name)
        lines.append(f"\tAnim_{pascal} AnimationID = 0x{anim_id:02X}")

    lines.append(")")
    lines.append("")
    lines.append(ANIMID_END)
    return "\n".join(lines)


def generate_mapping_block(all_animations: list) -> str:
    """Generate side mapping table and helper function."""
    # Find animations with logical_id and side
    left_mappings = []
    right_mappings = []

    for anim in all_animations:
        logical_id = anim.get("logical_id")
        side = anim.get("side")
        if logical_id is not None and side:
            # Find the logical animation name
            logical_anim = next((a for a in all_animations if a.get("id") == logical_id), None)
            if logical_anim:
                logical_name = to_pascal_case(logical_anim["name"])
                actual_name = to_pascal_case(anim["name"])
                if side == "left":
                    left_mappings.append((logical_name, actual_name))
                elif side == "right":
                    right_mappings.append((logical_name, actual_name))

    lines = [MAPPING_START]

    # Generate mapping table
    lines.append("var animationMapping = map[Address]map[AnimationID]AnimationID{")

    if left_mappings:
        lines.append("\tWorker_0: { // Left side")
        for logical, actual in left_mappings:
            lines.append(f"\t\tAnim_{logical}: Anim_{actual},")
        lines.append("\t},")

    if right_mappings:
        lines.append("\tWorker_1: { // Right side")
        for logical, actual in right_mappings:
            lines.append(f"\t\tAnim_{logical}: Anim_{actual},")
        lines.append("\t},")

    lines.append("}")
    lines.append("")

    # Generate helper function
    lines.append("// MapAnimation translates logical animation IDs to side-specific variants")
    lines.append("func MapAnimation(addr Address, id AnimationID) int {")
    lines.append("\tif mapping, ok := animationMapping[addr]; ok {")
    lines.append("\t\tif mapped, ok := mapping[id]; ok {")
    lines.append("\t\t\treturn int(mapped)")
    lines.append("\t\t}")
    lines.append("\t}")
    lines.append("\treturn int(id)")
    lines.append("}")
    lines.append("")
    lines.append(MAPPING_END)

    return "\n".join(lines)


def replace_between_markers(content: str, start_marker: str, end_marker: str, replacement: str) -> str:
    """Replace content between markers (inclusive)."""
    pattern = re.compile(
        rf"^[ \t]*{re.escape(start_marker)}.*?^[ \t]*{re.escape(end_marker)}",
        re.MULTILINE | re.DOTALL
    )
    return pattern.sub(replacement, content)


def validate_embedded_ids(animations: list) -> bool:
    """Validate that embedded animation IDs are sequential starting from 0."""
    ids = sorted([a.get("id", 0) for a in animations])
    expected = list(range(len(animations)))

    if ids != expected:
        print(f"ERROR: Embedded animation IDs must be sequential starting from 0")
        print(f"  Expected: {expected}")
        print(f"  Got: {ids}")
        return False
    return True


def validate_unique_ids(animations: list) -> bool:
    """Validate that all animation IDs are unique."""
    ids = [a.get("id", 0) for a in animations]
    if len(ids) != len(set(ids)):
        print(f"ERROR: Animation IDs must be unique")
        duplicates = [id for id in ids if ids.count(id) > 1]
        print(f"  Duplicates: {set(duplicates)}")
        return False
    return True


def update_main_go(animations: list) -> bool:
    """Update main.go with embed directives and loading code."""
    if not os.path.exists(MAIN_GO):
        print(f"ERROR: {MAIN_GO} not found")
        return False

    with open(MAIN_GO, "r") as f:
        content = f.read()

    if EMBED_START not in content:
        print(f"ERROR: Markers not found in main.go")
        return False

    embed_block = generate_embed_block(animations)
    load_block = generate_load_block(animations)
    append_block = generate_append_block(animations)

    content = replace_between_markers(content, EMBED_START, EMBED_END, embed_block)
    content = replace_between_markers(content, LOAD_START, LOAD_END, load_block)
    content = replace_between_markers(content, APPEND_START, APPEND_END, append_block)

    with open(MAIN_GO, "w") as f:
        f.write(content)

    print(f"Updated {MAIN_GO}")
    return True


def update_structs_go(all_animations: list) -> bool:
    """Update cmd/structs.go with AnimationID constants and mapping."""
    if not os.path.exists(STRUCTS_GO):
        print(f"ERROR: {STRUCTS_GO} not found")
        return False

    with open(STRUCTS_GO, "r") as f:
        content = f.read()

    if ANIMID_START not in content:
        print(f"ERROR: ANIMID markers not found in structs.go")
        return False

    if MAPPING_START not in content:
        print(f"ERROR: MAPPING markers not found in structs.go")
        return False

    animid_block = generate_animid_block(all_animations)
    mapping_block = generate_mapping_block(all_animations)

    content = replace_between_markers(content, ANIMID_START, ANIMID_END, animid_block)
    content = replace_between_markers(content, MAPPING_START, MAPPING_END, mapping_block)

    with open(STRUCTS_GO, "w") as f:
        f.write(content)

    print(f"Updated {STRUCTS_GO}")
    return True


def main():
    print(f"Loading configuration from {ANIMS_YAML}")

    if not os.path.exists(ANIMS_YAML):
        print(f"ERROR: {ANIMS_YAML} not found")
        sys.exit(1)

    with open(ANIMS_YAML, "r") as f:
        config = yaml.safe_load(f)

    all_animations = config.get("animations", [])
    embedded_animations = [a for a in all_animations if a.get("embed", False)]
    logical_animations = [a for a in all_animations if not a.get("embed", False)]

    if not embedded_animations:
        print("No animations with embed: true found")
        sys.exit(0)

    print(f"Found {len(embedded_animations)} embedded animation(s):")
    for a in sorted(embedded_animations, key=lambda x: x.get("id", 0)):
        side = f" (side: {a['side']})" if a.get("side") else ""
        print(f"  0x{a.get('id', 0):02X}: {a['name']}{side}")

    if logical_animations:
        print(f"Found {len(logical_animations)} logical animation(s):")
        for a in sorted(logical_animations, key=lambda x: x.get("id", 0)):
            print(f"  0x{a.get('id', 0):02X}: {a['name']}")

    # Validate IDs
    if not validate_unique_ids(all_animations):
        sys.exit(1)

    if not validate_embedded_ids(embedded_animations):
        sys.exit(1)

    # Update files
    success = True
    success = update_main_go(embedded_animations) and success
    success = update_structs_go(all_animations) and success

    if success:
        print("\nDone!")
    else:
        sys.exit(1)


if __name__ == "__main__":
    main()
