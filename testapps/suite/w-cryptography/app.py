from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import padding, rsa

# first output marks "imports done"
print("ready: w-cryptography")

key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
sig = key.sign(b"hello" * 1000, padding.PSS(mgf=padding.MGF1(hashes.SHA256()), salt_length=padding.PSS.MAX_LENGTH), hashes.SHA256())
key.public_key().verify(sig, b"hello" * 1000, padding.PSS(mgf=padding.MGF1(hashes.SHA256()), salt_length=padding.PSS.MAX_LENGTH), hashes.SHA256())
print(f"signature bytes={len(sig)} verified")
