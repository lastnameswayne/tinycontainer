package main

import "fmt"

func main() {
	fmt.Println("test")
	//standard slow docker flow
	//write your code
	//write your dockerfile
	//build the image, docker push
	//then on the host you run docker pull and start the image

	//a container image is everything that you need to run a container. so basically you need every package that
	//a python program needs. the python intepreter, modules, libraries, etc.
	//and this is slow, which modal is improving
	//Container images are composed of layers.
	//Each layer represented a set of file system changes that add, remove, or modify files

	//when the user runs 'tinycontainer run' the current package should be turned into a container
}
