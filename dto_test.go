package dto

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ==================================== Types for tests =======================

type Product struct {
	Name    string
	Country string
	Price   float32
}

type ProductRef struct {
	Product
	Link string
}

type ShoppingCart struct {
	Products []Product
}

type TaggedShoppingCart struct {
	Products map[string][]Product
}

type RawPassword = string

type User struct {
	Name     string
	Password RawPassword
}

type Order struct {
	Id string

	json.Marshaler   `dto:"ignore"`
	json.Unmarshaler `dto:"ignore"`
}

type OrderDto struct {
	Id string
}

// ==================================== Data for tests ========================

var commonProducts = []Product{
	{
		Name:    "Shirt",
		Price:   9.4,
		Country: "US",
	},
	{
		Name:    "Shoes",
		Price:   17.3,
		Country: "UK",
	},
	{
		Name:    "Hat",
		Price:   19.7,
		Country: "IT",
	},
	{
		Name:    "Bowtie",
		Price:   5.1,
		Country: "US",
	},
}

var cartByCountries = TaggedShoppingCart{
	Products: map[string][]Product{
		"Europe":  {commonProducts[1], commonProducts[2]},
		"America": {commonProducts[0], commonProducts[3]},
	},
}

// ==================================== Tests =================================

// Shallow structs with equal types
func TestSimple(t *testing.T) {
	var outNameAndPrice struct {
		Name  string
		Price float32
	}
	for _, product := range commonProducts {
		err := Map(&outNameAndPrice, product)
		assert.Nil(t, err)
		assert.Equal(t, product.Name, outNameAndPrice.Name)
		assert.Equal(t, product.Price, outNameAndPrice.Price)
	}
}

// Struct with simple conversion (float to int)
func TestSimpleConv(t *testing.T) {
	var outPriceInt struct {
		Price int
	}
	for _, product := range commonProducts {
		err := Map(&outPriceInt, product)
		assert.Nil(t, err)
		assert.Equal(t, int(product.Price), outPriceInt.Price)
	}
}

// ProductRef embeds Product
func TestEmbedded(t *testing.T) {
	var outNameAndLink struct {
		Name string
		Link string
	}
	for i, product := range commonProducts {
		productRef := ProductRef{
			Product: product,
			Link:    fmt.Sprintf("/p/%v", i),
		}
		Map(&outNameAndLink, productRef)
		assert.Equal(t, productRef.Name, outNameAndLink.Name)
		assert.Equal(t, productRef.Link, outNameAndLink.Link)
	}
}

// Map a slice of Products
func TestSlice(t *testing.T) {
	var outCart struct {
		Products []struct {
			Name string
		}
	}
	testCart := ShoppingCart{
		Products: commonProducts,
	}

	err := Map(&outCart, testCart)
	assert.Nil(t, err)

	assert.Equal(t, len(testCart.Products), len(outCart.Products))
	for i, product := range outCart.Products {
		assert.Equal(t, product.Name, testCart.Products[i].Name)
	}
}

// Map a map of Product slices tagged by strings
func TestMap(t *testing.T) {
	var outCart struct {
		Products map[string][]struct {
			Name string
		}
	}
	testCart := cartByCountries

	err := Map(&outCart, testCart)
	assert.Nil(t, err)

	assert.Equal(t, len(testCart.Products), len(outCart.Products))
	for key, list := range outCart.Products {
		testList, ok := testCart.Products[key]
		assert.True(t, ok)
		assert.Equal(t, len(testList), len(list))
		for i, product := range list {
			assert.Equal(t, testList[i].Name, product.Name)
		}
	}
}

// Map a map to a slice (its values)
func TestMapToSlice(t *testing.T) {
	var outCart struct {
		Products [][]struct {
			Name string
		}
	}
	testCart := cartByCountries

	err := Map(&outCart, testCart)
	assert.Nil(t, err)

	assert.Equal(t, len(testCart.Products), len(outCart.Products))
}

// Map a map of slices to a slice (flatten)
func TestMapSlicesToSlice(t *testing.T) {
	var outCart struct {
		Products []struct {
			Name string
		}
	}
	testCart := cartByCountries

	err := Map(&outCart, testCart)
	assert.Nil(t, err)

	tcLen := 0
	for _, v := range testCart.Products {
		tcLen += len(v)
	}
	assert.Equal(t, tcLen, len(outCart.Products))
}

// Dereference a pointer to map a struct
func TestPointerDeref(t *testing.T) {
	var outCart struct {
		Product struct {
			Name string
		}
	}
	var testCart struct {
		Product *Product
	}
	testCart.Product = &commonProducts[0]

	err := Map(&outCart, testCart)
	assert.Nil(t, err)

	assert.Equal(t, testCart.Product.Name, outCart.Product.Name)
}

// Conversion functions without errors and with mapper injection
func TestConversionFunc(t *testing.T) {
	type PasswordLen = int
	var outUser struct {
		Password PasswordLen
	}
	testUser := User{
		Name:     "Bob",
		Password: "Secret",
	}

	m := Mapper{}
	m.AddConvFunc(func(p RawPassword, im *Mapper) PasswordLen {
		assert.Equal(t, &m, im)
		return len(p)
	})

	err := m.Map(&outUser, testUser)
	assert.Nil(t, err)

	assert.Equal(t, len(testUser.Password), outUser.Password)
}

// Inspect functions without errors and with mapper injection that change data
func TestInspectFunc(t *testing.T) {
	type ProductDTO struct {
		Name      string
		Expensive bool
	}
	var outCart struct {
		Products []ProductDTO
	}
	testCart := ShoppingCart{
		Products: commonProducts,
	}

	m := Mapper{}
	m.AddInspectFunc(func(dto *ProductDTO, prod Product, im *Mapper) {
		assert.Equal(t, &m, im)
		assert.Equal(t, prod.Name, dto.Name)
		dto.Expensive = prod.Price > 10
	})

	err := m.Map(&outCart, testCart)
	assert.Nil(t, err)

	assert.Equal(t, len(testCart.Products), len(outCart.Products))
	for i, product := range outCart.Products {
		assert.Equal(t, testCart.Products[i].Price > 10, product.Expensive)
	}
}

func TestErrorNoValidMapping(t *testing.T) {
	var outProduct struct {
		Name int
	}
	err := Map(&outProduct, commonProducts[0])
	assert.ErrorIs(t, err, NoValidMappingError{
		ToType:   reflect.TypeOf(int(0)),
		FromType: reflect.TypeOf(string("")),
	})
}

// Propagate error from inspection function
func TestErrorPropagation(t *testing.T) {
	var outCart struct {
		Products []struct {
			Name string
		}
	}
	testCart := ShoppingCart{
		Products: commonProducts,
	}

	var testError = errors.New("Test error")

	m := Mapper{}
	m.AddInspectFunc(func(name *string) error {
		return testError
	})

	err := m.Map(&outCart, testCart)
	assert.Equal(t, testError, err)
}

// Test custom functions are not skipped for assignable types
func TestDontSkipFunctions(t *testing.T) {
	var outProduct Product
	testProduct := commonProducts[0]

	testError := errors.New("Called!")

	m := Mapper{}
	m.AddConvFunc(func(s string) (string, error) {
		return "", testError
	})
	err := m.Map(&outProduct, testProduct)
	assert.Equal(t, testError, err)
}

func TestPointerCases(t *testing.T) {
	{
		var fromProduct = struct {
			Prod *Product
		}{Prod: nil}
		var outProduct struct {
			Prod Product
		}
		Map(&outProduct, fromProduct)
		assert.Zero(t, outProduct.Prod)
	}
	{
		var fromProduct = struct {
			Prod Product
		}{Prod: commonProducts[0]}
		var outProduct struct {
			Prod *Product
		}
		Map(&outProduct, fromProduct)
		assert.NotNil(t, outProduct.Prod)
		assert.Equal(t, commonProducts[0], *outProduct.Prod)
	}
	{
		type SubProduct struct {
			Name string
		}
		var fromProduct = struct {
			Prod *Product
		}{Prod: &commonProducts[0]}
		var outProduct struct {
			Prod *SubProduct
		}
		Map(&outProduct, fromProduct)
		assert.Equal(t, commonProducts[0].Name, outProduct.Prod.Name)
	}
}

func TestStructureTagIgnoreCase(t *testing.T) {
	order := Order{Id: "test"}
	var outOrder OrderDto
	Map(&outOrder, order)
	assert.Equal(t, order.Id, outOrder.Id)
}

// ==================================== Benchmarks ============================

type benchCart = struct {
	Products []struct {
		Name string
	}
}

func benchMakeTestCart(size int) ShoppingCart {
	out := ShoppingCart{
		Products: make([]Product, size),
	}
	for i := 0; i < size; i++ {
		out.Products[i] = commonProducts[i%len(commonProducts)]
	}
	return out
}

// Benchmark without godto
func BenchmarkSimpleMap(b *testing.B) {
	var outCart benchCart
	testCart := benchMakeTestCart(1000)
	b.ResetTimer()

	outCart.Products = make([]struct{ Name string }, len(testCart.Products))
	for i, prod := range testCart.Products {
		outCart.Products[i].Name = prod.Name
	}
}

// Benchmark with godto
func BenchmarkDtoMap(b *testing.B) {
	var outCart benchCart
	testCart := benchMakeTestCart(1000)
	b.ResetTimer()

	Map(&outCart, testCart)
}
