import numpy as np
import tensorflow as tf

# first output marks "imports done"
print("ready: dl-tensorflow")

tf.config.threading.set_inter_op_parallelism_threads(1)
tf.config.threading.set_intra_op_parallelism_threads(1)
tf.random.set_seed(0)
model = tf.keras.Sequential([tf.keras.Input((32,)), tf.keras.layers.Dense(64, activation="relu"), tf.keras.layers.Dense(1)])
model.compile(optimizer="adam", loss="mse")
X, y = np.random.default_rng(0).standard_normal((2048, 32)), np.random.default_rng(1).standard_normal((2048, 1))
hist = model.fit(X, y, epochs=3, batch_size=256, verbose=0)
print(f"loss={hist.history['loss'][-1]:.4f}")
