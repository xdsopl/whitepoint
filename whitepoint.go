/*
whitepoint - Find brightest RGB value closest to d65 on Linux framebuffer
Written in 2017 by <Ahmet Inan> <xdsopl@gmail.com>
To the extent possible under law, the author(s) have dedicated all copyright and related and neighboring rights to this software to the public domain worldwide. This software is distributed without any warranty.
You should have received a copy of the CC0 Public Domain Dedication along with this software. If not, see <http://creativecommons.org/publicdomain/zero/1.0/>.
*/

package main
import (
	"os"
	"fmt"
	"math"
	"bufio"
	"errors"
	"strconv"
	"strings"
	"os/exec"
	"image"
	"image/color"
	"image/draw"
	"framebuffer"
)

func die(err interface{}) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

type XY struct { X, Y float64 }

func distance(a, b XY) float64 {
	return math.Sqrt((a.X-b.X)*(a.X-b.X)+(a.Y-b.Y)*(a.Y-b.Y))
}

func difference(a, b RGB) RGB {
	return RGB{a.R-b.R, a.G-b.G, a.B-b.B}
}

type RGB struct { R, G, B float64 }

func xy2rgb(xy XY) RGB {
	X := xy.X / xy.Y
	Y := 1.0
	Z := (1.0 - xy.X - xy.Y) / xy.Y
	R := 0.41847 * X - 0.15866 * Y - 0.082835 * Z
	G := -0.091169 * X + 0.25243 * Y + 0.015708 * Z
	B := 0.00092090 * X - 0.0025498 * Y + 0.17860 * Z
	return RGB{R, G, B}
}

func main() {
	fb, err := framebuffer.Open("/dev/fb0")
	if err != nil { die(err) }
	spotread := exec.Command("spotread", "-e", "-x")
	spotread_stdin, err := spotread.StdinPipe()
	if err != nil { die(err) }
	spotread_stdout, err := spotread.StdoutPipe()
	if err != nil { die(err) }
	scanner := bufio.NewScanner(spotread_stdout)
	err = spotread.Start()
	if err != nil { die(err) }

	measure := func() (XY, error) {
		n, err := spotread_stdin.Write([]byte{' '})
		if err != nil { return XY{0, 0}, err }
		if n != 1 { return XY{0, 0}, errors.New("Couldnt send one byte") }
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "Yxy: ") {
				tmp := strings.SplitN(scanner.Text(), "Yxy: ", 2)[1]
				fields := strings.Fields(tmp)
				x, err := strconv.ParseFloat(fields[1], 64)
				if err != nil { return XY{0, 0}, err }
				y, err := strconv.ParseFloat(fields[2], 64)
				if err != nil { return XY{0, 0}, err }
				return XY{x, y}, nil
			}
		}
		if scanner.Err() != nil { return XY{0, 0}, err }
		return XY{0, 0}, errors.New("Unexpected EOF")
	}

	adjust := func(r, g, b byte) {
		draw.Draw(fb, fb.Bounds(), &image.Uniform{color.RGBA{r, g, b, 255}}, image.ZP, draw.Src)
	}

	r := byte(255)
	g := byte(255)
	b := byte(255)

	adjust(r, g, b)
	now_xy, err := measure()
	if err != nil { die(err) }
	d65_xy := XY{0.31271, 0.32902}
	d65_rgb := xy2rgb(d65_xy)
	now_rgb := xy2rgb(now_xy)
	err_rgb := difference(d65_rgb, now_rgb)
	//fmt.Fprintln(os.Stderr, err_rgb)

	var c0 *byte
	var c1 *byte

	if err_rgb.R >= err_rgb.G && err_rgb.R >= err_rgb.B {
		fmt.Fprintln(os.Stderr, "adjusting green and blue")
		c0, c1 = &g, &b
	} else if err_rgb.G >= err_rgb.R && err_rgb.G >= err_rgb.B {
		fmt.Fprintln(os.Stderr, "adjusting red and blue")
		c0, c1 = &r, &b
	} else {
		fmt.Fprintln(os.Stderr, "adjusting red and green")
		c0, c1 = &r, &g
	}

	dis_xy := distance(d65_xy, now_xy)
	found := false
	for i := 0; i < 200; i++ {
		tmp := *c0
		*c0 -= 1
		adjust(r, g, b)
		xy, err := measure()
		if err != nil { die(err) }
		dis := distance(d65_xy, xy)
		fmt.Println(now_xy.X, now_xy.Y, d65_xy.X, d65_xy.Y, dis_xy)
		if dis > dis_xy + 0.0005 {
			*c0 = tmp
			if found { break }
			found = true
			c0, c1 = c1, c0
		} else {
			found = false
			dis_xy = dis
			now_xy = xy
		}
	}
	n, err := spotread_stdin.Write([]byte{'q', 'q'})
	if err != nil { die(err) }
	if n != 2 { die("Couldnt send two bytes") }
	err = spotread.Wait()
	if err != nil { die(err) }
}

