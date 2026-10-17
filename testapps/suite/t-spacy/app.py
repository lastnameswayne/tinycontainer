import spacy

# first output marks "imports done"
print("ready: t-spacy")

nlp = spacy.blank("en")
doc = nlp(" ".join(["Tinycontainer loads files lazily, so a cold start only fetches what Python imports."] * 2000))
print(f"tokens={len(doc)} unique={len({t.lower_ for t in doc})}")
