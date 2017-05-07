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

func naive(best_c0, best_c1 byte, best_xy, setpoint XY, measure func() (XY, error), adjust func(byte, byte)) {
	ca, cb := &best_c0, &best_c1
	best_dis := distance(setpoint, best_xy)
	found := false
	for i := 0; i < 200; i++ {
		tmp := *ca
		*ca -= 1
		adjust(best_c0, best_c1)
		xy, err := measure()
		if err != nil { die(err) }
		dis := distance(setpoint, xy)
		fmt.Println(xy.X, xy.Y, setpoint.X, setpoint.Y, dis)
		if dis > best_dis + 0.0005 {
			*ca = tmp
			if found { break }
			found = true
			ca, cb = cb, ca
		} else {
			found = false
			best_dis = dis
			best_xy = xy
		}
	}
	fmt.Println(best_xy.X, best_xy.Y, setpoint.X, setpoint.Y, best_dis)
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

	max_fix := byte(255)
	max_c0, max_c1 := max_fix, max_fix
	adj_rgb := color.RGBA{max_fix, max_c0, max_c1, 255}
	adj_c0, adj_c1 := &adj_rgb.G, &adj_rgb.B

	adjust := func(c0, c1 byte) {
		*adj_c0 = c0
		*adj_c1 = c1
		draw.Draw(fb, fb.Bounds(), &image.Uniform{adj_rgb}, image.ZP, draw.Src)
	}

	adjust(max_c0, max_c1)
	now_xy, err := measure()
	if err != nil { die(err) }
	d65_xy := XY{0.31271, 0.32902}
	d65_rgb := xy2rgb(d65_xy)
	now_rgb := xy2rgb(now_xy)
	err_rgb := difference(d65_rgb, now_rgb)
	//fmt.Fprintln(os.Stderr, err_rgb)

	if err_rgb.R >= err_rgb.G && err_rgb.R >= err_rgb.B {
		fmt.Fprintln(os.Stderr, "adjusting green and blue")
		adj_c0, adj_c1 = &adj_rgb.G, &adj_rgb.B
	} else if err_rgb.G >= err_rgb.R && err_rgb.G >= err_rgb.B {
		fmt.Fprintln(os.Stderr, "adjusting red and blue")
		adj_c0, adj_c1 = &adj_rgb.R, &adj_rgb.B
	} else {
		fmt.Fprintln(os.Stderr, "adjusting red and green")
		adj_c0, adj_c1 = &adj_rgb.R, &adj_rgb.G
	}

	naive(max_c0, max_c1, now_xy, d65_xy, measure, adjust)

	n, err := spotread_stdin.Write([]byte{'q', 'q'})
	if err != nil { die(err) }
	if n != 2 { die("Couldnt send two bytes") }
	err = spotread.Wait()
	if err != nil { die(err) }
}

