#!/usr/bin/env python3
"""
Compiles all animations defined in anims.yaml and copies them to the animations folder.
"""

import os
import sys
import shutil
import subprocess
import yaml

# Paths relative to project root
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ANIMS_YAML = os.path.join(PROJECT_ROOT, "anims.yaml")
COMPILER_DIR = os.path.join(PROJECT_ROOT, "compiler")
COMPILER_SCRIPT = os.path.join(COMPILER_DIR, "compiler", "process.py")
SOURCE_ANIMS_DIR = os.path.join(COMPILER_DIR, "animations")
COMPILED_DIR = os.path.join(SOURCE_ANIMS_DIR, "compiled")
OUTPUT_DIR = os.path.join(PROJECT_ROOT, "animations")


def compile_animation(anim_config: dict) -> bool:
    """Compile a single animation with the given configuration."""
    name = anim_config["name"]
    source = anim_config["source"]
    rotation = anim_config.get("rotation", 0)
    matrix_width = anim_config.get("matrix_width", 16)
    matrix_height = anim_config.get("matrix_height", 16)
    brightness = anim_config.get("brightness", 1.0)

    source_path = os.path.join(SOURCE_ANIMS_DIR, source)

    if not os.path.exists(source_path):
        print(f"  ERROR: Source not found: {source_path}")
        return False

    # Build the command
    cmd = [
        sys.executable,
        COMPILER_SCRIPT,
        source_path,
        "--output-dir", COMPILED_DIR,
        "--rotation", str(rotation),
        "--matrix_width", str(matrix_width),
        "--matrix_height", str(matrix_height),
        "--brightness", str(brightness),
        "--pack",
    ]

    print(f"  Compiling {source} -> {name}.animbyte")
    print(f"    rotation={rotation}, matrix={matrix_width}x{matrix_height}, brightness={brightness}")

    result = subprocess.run(cmd, cwd=COMPILER_DIR, capture_output=True, text=True)

    if result.returncode != 0:
        print(f"  ERROR: Compilation failed")
        print(result.stderr)
        return False

    # The compiler outputs <source_basename>.animbyte
    # We need to rename it to <name>.animbyte
    source_basename = os.path.splitext(source)[0]
    compiled_file = os.path.join(COMPILED_DIR, f"{source_basename}.animbyte")
    output_file = os.path.join(OUTPUT_DIR, f"{name}.animbyte")

    if not os.path.exists(compiled_file):
        print(f"  ERROR: Expected output not found: {compiled_file}")
        return False

    # Copy (not move) to output directory with the correct name
    shutil.copy2(compiled_file, output_file)
    print(f"  Copied to {output_file}")

    return True


def main():
    print(f"Loading configuration from {ANIMS_YAML}")

    if not os.path.exists(ANIMS_YAML):
        print(f"ERROR: {ANIMS_YAML} not found")
        sys.exit(1)

    with open(ANIMS_YAML, "r") as f:
        config = yaml.safe_load(f)

    all_animations = config.get("animations", [])

    # Filter to only animations that need compilation (embed: true)
    animations = [a for a in all_animations if a.get("embed", False)]

    if not animations:
        print("No animations to compile (none with embed: true)")
        sys.exit(0)

    # Ensure output directories exist
    os.makedirs(COMPILED_DIR, exist_ok=True)
    os.makedirs(OUTPUT_DIR, exist_ok=True)

    print(f"Compiling {len(animations)} animation(s)...\n")

    success_count = 0
    fail_count = 0

    for anim in animations:
        name = anim.get("name", "unknown")
        print(f"[{name}]")

        if compile_animation(anim):
            success_count += 1
        else:
            fail_count += 1
        print()

    print(f"Done! {success_count} succeeded, {fail_count} failed")

    if fail_count > 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
