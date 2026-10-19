# MTProxyL templates

Source: https://github.com/Liafanx/MTProxyL/tree/6d7021c9ad0b29d15cb0c4a7b3098061ab012bee/templates_html

The HTML sources are vendored under the accompanying MIT license, with trailing
whitespace normalized.
The panel removes remote font links and publishes inline styles/scripts as
local assets when deploying these pages for Telemt's static-site CSP.
The file-manager template is excluded because it requires remote Tailwind
and Lucide scripts; this collection has no runtime CDN dependencies.
