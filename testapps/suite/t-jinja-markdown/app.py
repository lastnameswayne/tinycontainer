import jinja2
import markdown

# first output marks "imports done"
print("ready: t-jinja-markdown")

tmpl = jinja2.Template("# {{ title }}\n\n{% for i in items %}- item **{{ i }}**\n{% endfor %}")
md = tmpl.render(title="Report", items=range(5000))
html = markdown.markdown(md)
print(f"markdown chars={len(md)} html chars={len(html)}")
