## Dto mapper

<a href="https://pkg.go.dev/github.com/dranikpg/dto-mapper"><img src="https://godoc.org/github.com/dranikpg/dto-mapper?status.svg" /></a>
[![Go Report Card](https://goreportcard.com/badge/github.com/dranikpg/dto-mapper?fix-cache-1)](https://goreportcard.com/report/github.com/dranikpg/dto-mapper)

dto-mapper is an easy-to-use library for complex struct mapping. It's intended for the creation of [data transfer objects](https://en.wikipedia.org/wiki/Data_transfer_object), hence the name.

When working with database relations and ORMs, you often fetch more data than needed. One could create subtypes for all kinds of queries, but this is not suitable for quick prototyping. Tools like [go-funk](https://github.com/thoas/go-funk) and [structs](https://github.com/fatih/structs) make your mapping code less verbose, but _you still have to write it_.

dto-mapper requries __only a declaration__ and contraty to many other struct mappers uses __only name-based field resolution__, works with __arbitrary deep__ structures, slices, maps, poiners, embedded structs, supports custom conversion functions, error handling... you name it!

### Simple example

You just have to declare the values you want to extract and dto does the rest.

```go
func GetUserPosts(c *gin.Context) {
    var userDto struct {
        Name  string
        Posts []struct {
          Id    int
          Title string
        }
    }
    userModel := LoadUserWithPosts(userId)
    dto.Map(&userDto, userModel)
    c.JSON(200, userDto)
}
```

### Installation

```
go get github.com/dranikpg/dto-mapper
```

### More examples

##### Slices, maps and structs

dto works its way down recursively through struct fields, slices and maps.

```go
var shoppingCart struct {
    GroupedProducts map[string][]struct {
        Name      string
        Warehouse struct {
            Location string
        }
    }
}
```

Maps can be converted into slices of their values. 
What's more, a map of slices can be converted into a single slice.

```go
var allProducts []Product = nil
var groupedProducts map[string][]Product = GetProducts()
dto.Map(&allProducts, groupedProducts)
```

##### Emedded structs and pointers

Embedded struct fields are included. Pointers are automatically dereferenced.

```go
type User struct {
    gorm.Model
    CompanyRefer int
    Company      *Company `gorm:"foreignKey:CompanyRefer"`
}

type UserDto struct {
    ID      int
    Company struct {
        Name string
    }
}
```

##### Ignored fields

If you need to ignore any of the structure fields, you can apply the structure tag - dto:ignore

```go
type Order struct {
	Id string

	json.Marshaler   `dto:"ignore"`
	json.Unmarshaler `dto:"ignore"`
}

type OrderDto struct {
	Id string
}
```

#### Mapper instances

Local mapper instances can be used to add conversion and inspection functions. Mappers don't change their internal state during mapping, so they can be reused at any time.

```go
mapper := dto.Mapper{}
mapper.Map(&to, from)
```

##### Conversion functions

They are used to convert one type into another and have the highest priority, however they are not applied to fields of directly assignable structs. The second argument is the current mapper instance and is optional.

```go
mapper.AddConvFunc(func(p RawPassword, mapper *Mapper) PasswordHash {
    return hash(p)
})
```

##### Inspection functions 

Those are triggered _after_ a value has been successfully mapped. The value is **always taken by pointer**. Likewise to conversion functions, they are not called for fields of directly assignable structs.

```go
mapper.AddInspectFunc(func(dto *UserDto) {
    dto.Link = GenerateLink(dto.ID)
})
```

They can also be defined with a specific source type. The last argument is optional.

```go
mapper.AddInspectFunc(func(dto *UserDto, user User, mapper *Mapper) {
    dto.Online = IsRecent(user.LastSeen)
})
```

##### Error handling

* Both conversion and inspection functions can return errors by returning `(value, error)` and `error` respectively
* If dto failed to map one value onto another, it returns `ErrNoValidMapping`
* dto silently skips struct fields it found no source for (i.e. no fields with the same name)

Mapping stops as soon as an error is encountered.

```go
mapper.AddInspectFunc(func(dto *UserDto) error {
    if len(dto.Link) == 0 {
        return errors.New("malformed link")
    }
    return nil
})

err := mapper.Map(&to, from) // error: malformed link
```

### Performance

Dto is based on reflection and therefore much slower than handwritten mapping code. 
Furthermore, using custom functions disables direct assignment of composite types. 

### Contributing

Missing a common use case? Feel free to contribute!
