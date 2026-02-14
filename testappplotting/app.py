import numpy as np
import plotext as plt

def main():
    x = np.linspace(0, 2 * np.pi, 100)
    y = np.sin(x)

    plt.plot(x, y)
    plt.title("Sine Wave")
    plt.show()


if __name__ == "__main__":
    main()
