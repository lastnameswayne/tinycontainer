import numpy as np
from skimage import filters, measure, morphology

# first output marks "imports done"
print("ready: i-skimage")

img = np.random.default_rng(0).random((1024, 1024))
mask = filters.gaussian(img, sigma=4) > 0.5
labels = measure.label(morphology.binary_opening(mask))
print(f"regions={labels.max()}")
