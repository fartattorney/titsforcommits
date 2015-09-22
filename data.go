package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

func ReadData(path string) []byte {
	d, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println(err)
		return []byte(``)
	}

	return d
}

func WriteData(path string, data []byte) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Println(err)
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		fmt.Println(err)
	}
}
