import django
from django.conf import settings

# first output marks "imports done"
print("ready: w-django")

settings.configure(DEBUG=False, INSTALLED_APPS=[], TEMPLATES=[{"BACKEND": "django.template.backends.django.DjangoTemplates"}])
django.setup()
from django.template import Context, Engine

tmpl = Engine.get_default().from_string("{% for r in rows %}<tr><td>{{ r.0 }}</td><td>{{ r.1|floatformat:2 }}</td></tr>{% endfor %}")
html = tmpl.render(Context({"rows": [(i, i / 7) for i in range(5000)]}))
print(f"html chars={len(html)}")
