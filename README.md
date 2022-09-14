# Base62

[![GoDoc](https://godoc.org/github.com/yihleego/base62?status.svg)](https://godoc.org/github.com/yihleego/base62)
[![Go Report Card](https://goreportcard.com/badge/github.com/yihleego/base62)](https://goreportcard.com/report/github.com/yihleego/base62)

The base62 encoding scheme uses 62 characters. The characters consist of the capital letters A-Z, the lower case letters a-z and the numbers 0–9. It is a binary-to-text encoding schemes that represent binary data in an ASCII string format.

## Base62 Alphabet

```
0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz
```

## Base62 table

|Decimal|Binary|Base62| |Decimal|Binary|Base62| |Decimal|Binary|Base62| |Decimal|Binary|Base62|
|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
|0|000000|0| |16|010000|G| |32|100000|W| |48|110000|m|
|1|000001|1| |17|010001|H| |33|100001|X| |49|110001|n|
|2|000010|2| |18|010010|I| |34|100010|Y| |50|110010|o|
|3|000011|3| |19|010011|J| |35|100011|Z| |51|110011|p|
|4|000100|4| |20|010100|K| |36|100100|a| |52|110100|q|
|5|000101|5| |21|010101|L| |37|100101|b| |53|110101|r|
|6|000110|6| |22|010110|M| |38|100110|c| |54|110110|s|
|7|000111|7| |23|010111|N| |39|100111|d| |55|110111|t|
|8|001000|8| |24|011000|O| |40|101000|e| |56|111000|u|
|9|001001|9| |25|011001|P| |41|101001|f| |57|111001|v|
|10|001010|A| |26|011010|Q| |42|101010|g| |58|111010|w|
|11|001011|B| |27|011011|R| |43|101011|h| |59|111011|x|
|12|001100|C| |28|011100|S| |44|101100|i| |60|111100|y|
|13|001101|D| |29|011101|T| |45|101101|j| |61|111101|z|
|14|001110|E| |30|011110|U| |46|101110|k| |  |     |  |
|15|001111|F| |31|011111|V| |47|101111|l| |  |     |  |

See: [Wikipedia Base62](https://en.wikipedia.org/wiki/Base62)

## Usage

### Encode

```go
src := []byte("Hello, World!")
encoded := base62.StdEncoding.Encode(src)
// {49, 119, 74, 102, 114, 122, 118, 100, 98, 116, 88, 85, 79, 108, 85, 106, 85, 102}
```

### EncodeToString

```go
src := []byte("Hello, World!")
encoded := base62.StdEncoding.EncodeToString(src)
// 1wJfrzvdbtXUOlUjUf
```

### Decode

```go
src := []byte{49, 119, 74, 102, 114, 122, 118, 100, 98, 116, 88, 85, 79, 108, 85, 106, 85, 102}
decoded, err := base62.StdEncoding.Decode(src)
// {72, 101, 108, 108, 111, 44, 32, 87, 111, 114, 108, 100, 33}
str := string(decoded)
// Hello, World!
```

### DecodeString

```go
src := "1wJfrzvdbtXUOlUjUf"
decoded, err := base62.StdEncoding.DecodeString(src)
// {72, 101, 108, 108, 111, 44, 32, 87, 111, 114, 108, 100, 33}
str := string(decoded)
// Hello, World!
```

### Custom alphabet

```go
// 62-byte string
encoding := base62.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

src := []byte("Hello, World!")
encoded := encoding.Encode(src)
// {66, 54, 84, 112, 49, 57, 53, 110, 108, 51, 104, 101, 89, 118, 101, 116, 101, 112}
```