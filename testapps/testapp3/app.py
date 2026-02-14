import numpy as np

def main():
    print("hello world")
    arr = np.array([1, 2, 3, 4, 5])
    print("Sum:", arr.sum())
    print("Mean:", arr.mean())
    print(inference(116,-1))
    print(inference(0,0))


def inference(x1,x2):
    model_weights = np.array([-1.0,1.0])
    return float(model_weights @ np.array([x1,x2]))

if __name__ == "__main__":
    main()
