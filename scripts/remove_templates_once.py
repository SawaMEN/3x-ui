from pathlib import Path
import json


def replace_exact(path: str, old: str, new: str = "") -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected exactly one match, got {count}: {old!r}")
    p.write_text(text.replace(old, new, 1))


# Frontend navigation and route.
replace_exact("frontend/src/layouts/AppSidebar.tsx", "  FileProtectOutlined,\n")
replace_exact(
    "frontend/src/layouts/AppSidebar.tsx",
    "  | 'telemt'\n  | 'tool'\n  | 'templates';",
    "  | 'telemt'\n  | 'tool';",
)
replace_exact("frontend/src/layouts/AppSidebar.tsx", "  templates: FileProtectOutlined,\n")
replace_exact(
    "frontend/src/layouts/AppSidebar.tsx",
    "      { key: '/templates', icon: 'templates' as IconName, title: t('menu.templates') },\n",
)
replace_exact(
    "frontend/src/routes.tsx",
    "const TemplatesPage = lazy(() => import('@/pages/templates/TemplatesPage'));\n",
)
replace_exact(
    "frontend/src/routes.tsx",
    "      { path: 'templates', element: withSuspense(<TemplatesPage />) },\n",
)

# Backend route and auto-migrated model registration.
replace_exact(
    "internal/web/controller/api.go",
    "\ttemplates := api.Group(\"/templates\")\n\tNewTemplateController(templates)\n\n",
)
replace_exact("internal/database/db.go", "\t\t&model.LocalTemplate{},\n")

# API docs source section.
p = Path("frontend/src/pages/api-docs/endpoints.ts")
text = p.read_text()
begin = text.find("  {\n    id: 'templates',")
end = text.find("  {\n    id: 'routing-presets',", begin)
if begin < 0 or end < 0:
    raise SystemExit("frontend/src/pages/api-docs/endpoints.ts: templates section not found")
p.write_text(text[:begin] + text[end:])

# Locale keys used only by the removed page/menu.
for locale in ("internal/web/translation/en-US.json", "internal/web/translation/ru-RU.json"):
    p = Path(locale)
    data = json.loads(p.read_text())
    try:
        del data["menu"]["templates"]
        del data["pages"]["templates"]
    except KeyError as exc:
        raise SystemExit(f"{locale}: expected templates key missing: {exc}") from exc
    p.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")

# Files dedicated to the feature.
for filename in (
    "frontend/src/pages/templates/TemplatesPage.css",
    "frontend/src/pages/templates/TemplatesPage.tsx",
    "frontend/src/test/templates-search.test.tsx",
    "internal/database/model/local_template.go",
    "internal/web/controller/template.go",
    "internal/web/controller/template_id_test.go",
    "internal/web/service/template.go",
    "internal/web/service/template_test.go",
    "docs/content/docs/en/reference/api/templates.mdx",
):
    p = Path(filename)
    if not p.exists():
        raise SystemExit(f"expected feature file missing: {filename}")
    p.unlink()

# Remove templates from the docs copy of OpenAPI as well; frontend copy is regenerated.
docs_spec = Path("docs/public/openapi.json")
if docs_spec.exists():
    data = json.loads(docs_spec.read_text())
    paths = data.get("paths", {})
    for key in list(paths):
        if "/templates/" in key or key.endswith("/templates"):
            del paths[key]
    components = data.get("components", {}).get("schemas", {})
    for key in list(components):
        if key.lower().startswith("template"):
            del components[key]
    if isinstance(data.get("tags"), list):
        data["tags"] = [t for t in data["tags"] if str(t.get("name", "")).lower() != "templates"]
    docs_spec.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
