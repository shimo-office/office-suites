"""Release asset names shared by the packaging and verification tools."""
ASSETS = frozenset(
    f'shimo-net_{system}_{arch}' + ('.exe' if system == 'windows' else '')
    for system in ['darwin', 'linux', 'windows'] for arch in ['amd64', 'arm64']
)
