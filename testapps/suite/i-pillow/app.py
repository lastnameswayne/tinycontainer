import io
from PIL import Image, ImageFilter

# first output marks "imports done"
print("ready: i-pillow")

img = Image.effect_mandelbrot((1024, 1024), (-2.0, -1.5, 1.0, 1.5), 100).convert("RGB")
out = img.filter(ImageFilter.GaussianBlur(3)).resize((512, 512))
buf = io.BytesIO()
out.save(buf, format="JPEG", quality=85)
print(f"jpeg bytes={len(buf.getvalue())}")
