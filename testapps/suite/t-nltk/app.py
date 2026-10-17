import collections
from nltk.tokenize import wordpunct_tokenize
from nltk.util import ngrams

# first output marks "imports done"
print("ready: t-nltk")

text = "the quick brown fox jumps over the lazy dog, and the dog sleeps. " * 20_000
tokens = wordpunct_tokenize(text)
top = collections.Counter(ngrams(tokens, 2)).most_common(2)
print(f"tokens={len(tokens)} top={top}")
