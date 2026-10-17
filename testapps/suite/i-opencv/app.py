import numpy as np
import cv2

# first output marks "imports done"
print("ready: i-opencv")

img = (np.random.default_rng(0).random((1080, 1920)) * 255).astype(np.uint8)
edges = cv2.Canny(cv2.GaussianBlur(img, (7, 7), 2), 50, 150)
print(f"edge pixels={int((edges > 0).sum())}")
