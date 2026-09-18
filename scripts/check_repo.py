"""Check local documentation links and common accidental publication artifacts."""

from pathlib import Path
import re
import sys
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parent.parent
EXCLUDED = {'.git', 'bin', 'reports', '.compact-data', '__pycache__', '.codebase-memory'}


def repository_files():
    return sorted(p for p in ROOT.rglob('*') if p.is_file() and not EXCLUDED.intersection(p.relative_to(ROOT).parts))


def main():
    errors = []
    files = repository_files()
    for path in files:
        relative = path.relative_to(ROOT)
        if path.name == '.env' or path.suffix in {'.pem', '.key'}:
            errors.append(f'{relative}: private configuration or key file')
        try:
            text = path.read_text()
        except UnicodeDecodeError:
            if relative.parent == Path('demo/assets') and path.suffix in {'.gif', '.mp4', '.png'}:
                continue
            errors.append(f'{relative}: unexpected binary file')
            continue
        if re.search(r'apikey_[0-9a-f]{32}_[0-9a-f]{64}', text):
            errors.append(f'{relative}: possible TypeSafe credential')
        if re.search(r'/Users/[A-Za-z0-9_.-]+/', text):
            errors.append(f'{relative}: personal absolute path')
        if path.suffix != '.md':
            continue
        for target in re.findall(r'\[[^\]]*\]\(([^\s)]+)\)', text):
            if target.startswith(('https://', 'http://', 'mailto:', '#')):
                continue
            local = unquote(target.split('#', 1)[0])
            if not (path.parent / local).exists():
                errors.append(f'{relative}: broken local link {target}')
    for error in errors:
        print(error, file=sys.stderr)
    if errors:
        return 1
    print(f'Checked {len(files)} files: local links and publication checks passed.')
    print('This focused check is not a comprehensive secret scanner.')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
