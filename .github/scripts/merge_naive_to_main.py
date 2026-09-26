from pathlib import Path


def once(text: str, old: str, new: str, label: str) -> str:
    n = text.count(old)
    if n != 1:
        raise SystemExit(f"{label}: expected one match, found {n}")
    return text.replace(old, new, 1)


# Register the standalone controller while preserving the current main router.
path = Path("internal/web/controller/api.go")
text = path.read_text()
if "NewNaiveProxyController(api)" not in text:
    old = '\tNewTelemtController(telemt, a.settingService)\n\n\troutingPresets := api.Group("/xray/routingPresets")'
    new = '\tNewTelemtController(telemt, a.settingService)\n\n\t// Standalone NaiveProxy is used only when Xray is selected.\n\tNewNaiveProxyController(api)\n\n\troutingPresets := api.Group("/xray/routingPresets")'
    text = once(text, old, new, "register NaiveProxy controller")
path.write_text(text)


# Caddy-Naive is distributed as .tar.xz. Keep main's safe full-upgrade policy
# and add the decompressor. Arch installs missing dependencies in the same -Syu
# transaction, avoiding an unsupported partial upgrade.
path = Path("internal/util/systemupdate/manager_linux.go")
text = path.read_text()
for old, new in {
    '"cron", "curl", "tar", "tzdata"': '"cron", "curl", "tar", "xz-utils", "tzdata"',
    '"cronie", "curl", "tar", "tzdata"': '"cronie", "curl", "tar", "xz", "tzdata"',
    '"cron", "curl", "tar", "timezone"': '"cron", "curl", "tar", "xz", "timezone"',
    '"dcron", "curl", "tar", "tzdata"': '"dcron", "curl", "tar", "xz", "tzdata"',
}.items():
    text = text.replace(old, new)
old = '\tupgradeArgs := packageUpgradeCommand(info.manager)\n\toutput, upgradeErr := runCommand(updateCtx, upgradeArgs[0], upgradeArgs[1:]...)'
new = '\tupgradeArgs := packageUpgradeCommand(info.manager)\n\tif info.manager == "pacman" && len(missing) > 0 {\n\t\tupgradeArgs = packageUpgradeCommand(info.manager, missing)\n\t}\n\toutput, upgradeErr := runCommand(updateCtx, upgradeArgs[0], upgradeArgs[1:]...)'
text = once(text, old, new, "safe Arch dependency install")
text = text.replace(
    "используют curl, tar, ca-certificates, openssl и socat",
    "используют curl, tar, xz, ca-certificates, openssl и socat",
)
path.write_text(text)


# Integrate standalone Naive into current main's System Updates UI while
# retaining the newer Pingtunnel/TrustTunnel functionality.
path = Path("frontend/src/pages/settings/SystemUpdateModal.tsx")
text = path.read_text()

old = "const isInstallableDependency = (key: DependencyKey) =>\n  key === 'sudoku' || key === 'pingtunnel' || key === 'trusttunnel';"
new = """const isDependencyInstallable = (dependency: DependencyStatus) =>
  dependency.key === 'sudoku' ||
  dependency.key === 'pingtunnel' ||
  dependency.key === 'trusttunnel' ||
  (dependency.key === 'naiveproxy' && dependency.source === 'xray');

const dependencyNeedsAction = (dependency: DependencyStatus) => {
  if (!dependency.availableVersion) return false;
  if (isDependencyInstallable(dependency)) {
    return !dependency.installed || dependency.updateAvailable;
  }
  return dependency.installed && dependency.updateAvailable;
};"""
text = once(text, old, new, "dependency helpers")
text = text.replace(
    "isInstallableDependency(dependency.key)",
    "isDependencyInstallable(dependency)",
)

old = """      singBoxVersions,
      sudokuStatus,
      externalVPNStatus,"""
new = """      singBoxVersions,
      naiveProxyStatus,
      sudokuStatus,
      externalVPNStatus,"""
text = once(text, old, new, "status destructuring")

sudoku = """      safeGet<{
        installed?: boolean;
        version?: string;
        latestVersion?: string;
        updateAvailable?: boolean;
      }>('/panel/api/setting/sudoku/status'),"""
naive = """      safeGet<{
        installed?: boolean;
        version?: string;
        latestVersion?: string;
        updateAvailable?: boolean;
      }>('/panel/api/naiveproxy/status'),
"""
text = once(text, sudoku, naive + sudoku, "standalone status request")

marker = "    const sudokuCurrent = sudokuStatus.success ? sudokuStatus.obj?.version || '' : '';"
values = """    const standaloneNaiveInstalled =
      naiveProxyStatus.success === true && naiveProxyStatus.obj?.installed === true;
    const standaloneNaiveCurrent = naiveProxyStatus.success
      ? naiveProxyStatus.obj?.version || ''
      : '';
    const standaloneNaiveLatest = naiveProxyStatus.success
      ? naiveProxyStatus.obj?.latestVersion || ''
      : '';
    const naiveInstalled =
      coreType === 'sing-box'
        ? singBoxInstalled
        : coreType === 'xray'
          ? standaloneNaiveInstalled
          : false;
    const naiveCurrent =
      coreType === 'sing-box'
        ? singBoxCurrent
        : coreType === 'xray'
          ? standaloneNaiveCurrent
          : '';
    const naiveLatest =
      coreType === 'sing-box'
        ? stableSingBoxVersion
        : coreType === 'xray'
          ? standaloneNaiveLatest
          : '';

"""
text = once(text, marker, values + marker, "standalone Naive versions")

old = """      {
        key: 'naiveproxy',
        label: 'NaiveProxy',
        installed: coreType === 'sing-box' && singBoxInstalled,
        installedVersion: coreType === 'sing-box' ? singBoxCurrent : '',
        availableVersion: coreType === 'sing-box' ? stableSingBoxVersion : '',
        updateAvailable: Boolean(
          coreType === 'sing-box' &&
            singBoxInstalled &&
            singBoxCurrent &&
            stableSingBoxVersion &&
            versionsDiffer(singBoxCurrent, stableSingBoxVersion),
        ),
      },"""
new = """      {
        key: 'naiveproxy',
        label: 'NaiveProxy',
        source: coreType || undefined,
        installed: naiveInstalled,
        installedVersion: naiveCurrent,
        availableVersion: naiveLatest,
        updateAvailable: Boolean(
          naiveInstalled && naiveCurrent && naiveLatest && versionsDiffer(naiveCurrent, naiveLatest),
        ),
      },"""
text = once(text, old, new, "Naive dependency row")

old = """      case 'naiveproxy':
        return (await HttpUtil.post(
          `/panel/api/setting/singbox/install/${encodeURIComponent(dependency.availableVersion)}`,
        )) as ApiMsg<unknown>;"""
new = """      case 'naiveproxy':
        if (dependency.source === 'xray') {
          return (await HttpUtil.post('/panel/api/naiveproxy/update')) as ApiMsg<unknown>;
        }
        return (await HttpUtil.post(
          `/panel/api/setting/singbox/install/${encodeURIComponent(dependency.availableVersion)}`,
        )) as ApiMsg<unknown>;"""
text = once(text, old, new, "Naive update request")

old = """    if (
      !dependency.availableVersion ||
      (!dependency.installed && !isDependencyInstallable(dependency))
    )
      return;"""
text = once(text, old, "    if (!dependencyNeedsAction(dependency)) return;", "update guard")

old = """      const pending = dependencies.filter((dependency) => {
        if (!dependency.availableVersion) return false;
        if (isDependencyInstallable(dependency)) {
          return !dependency.installed || dependency.updateAvailable;
        }
        return dependency.installed && dependency.updateAvailable;
      });"""
text = once(text, old, "      const pending = dependencies.filter(dependencyNeedsAction);", "pending list")

old = """          dependency.key === 'naiveproxy'
            ? 'sing-box'
            : dependency.key === 'hysteria2'"""
new = """          dependency.key === 'naiveproxy'
            ? dependency.source === 'xray'
              ? 'naiveproxy'
              : 'sing-box'
            : dependency.key === 'hysteria2'"""
text = once(text, old, new, "update target")

old = """  const componentUpdatesAvailable = dependencies.some(
    (dependency) =>
      dependency.updateAvailable ||
      (isDependencyInstallable(dependency) &&
        !dependency.installed &&
        Boolean(dependency.availableVersion)),
  );"""
text = once(
    text,
    old,
    "  const componentUpdatesAvailable = dependencies.some(dependencyNeedsAction);",
    "component status",
)

old = "                        NaiveProxy обновляется вместе с sing-box, поскольку Naive является встроенным протоколом sing-box."
new = """                        {dependency.source === 'xray'
                          ? 'При Xray NaiveProxy используется как отдельный бинарник и обновляется напрямую из официальных релизов klzgrad/forwardproxy.'
                          : 'При sing-box NaiveProxy используется из встроенной реализации sing-box и обновляется вместе с sing-box.'}"""
text = once(text, old, new, "Naive UI explanation")
path.write_text(text)


# Keep API documentation and the live router in lockstep.
path = Path("frontend/src/pages/api-docs/endpoints.ts")
text = path.read_text()
if "/panel/api/naiveproxy/status" not in text:
    marker = "export const sections: readonly Section[] = [\n"
    section = """  {
    id: 'naiveproxy',
    title: 'NaiveProxy',
    description:
      'Manage the standalone Caddy-Naive sidecar used when Xray is the selected core.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/naiveproxy/status',
        summary: 'Return standalone NaiveProxy installation and runtime status.',
      },
      {
        method: 'POST',
        path: '/panel/api/naiveproxy/update',
        summary: 'Install or update standalone NaiveProxy from the official release.',
      },
    ],
  },
"""
    text = once(text, marker, marker + section, "Naive API docs")
path.write_text(text)
