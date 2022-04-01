package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ordered_item struct {
	Item_name     string `json:"item_name,omitempty" bson:"item_name,omitempty"`
	Item_quantity int32  `json:"item_quantity,omitempty" bson:"item_quantity,omitempty"`
}

type Order struct {
	ID            int32          `json:"_id,omitempty" bson:"_id,omitempty"`
	MenuID        string         `json:"menuid,omitempty" bson:"menuid,omitempty"`
	Ordered_items []ordered_item `json:"ordered_items,omitempty" bson:"ordered_items,omitempty"`
	Total_cost    int32          `json:"total_cost,omitempty" bson:"total_cost,omitempty"`
	Order_date    string         `json:"order_date,omitempty" bson:"order_date,omitempty"`
	Zip           string         `json:"zip,omitempty" bson:"zip,omitempty"`
}

type items struct {
	Item_name string `json:"item_name,omitempty" bson:"item_name,omitempty"`
	Item_cost int32  `json:"item_cost,omitempty" bson:"item_cost,omitempty"`
}

type Menu struct {
	ID         int32              `json:"_id,omitempty" bson:"_id,omitempty"`
	Name       string             `json:"name,omitempty" bson:"name,omitempty"`
	ChefID     int32              `json:"chefID,omitempty" bson:"chefID,omitempty"`
	Rating     int32              `json:"rating,omitempty" bson:"rating,omitempty"`
	Datefrom   primitive.DateTime `json:"datefrom,omitempty" bson:"datefrom,omitempty"`
	Dateto     primitive.DateTime `json:"dateto,omitempty" bson:"dateto,omitempty"`
	Menu_items []items            `json:"items,omitempty" bson:"items,omitempty"`
}

type Connection struct {
	order *mongo.Collection
	menu  *mongo.Collection
	chef  *mongo.Collection
	user  *mongo.Collection
}

func (connection Connection) GetMenusEndpoint(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("content-type", "application/json")
	var menus []Menu
	zip, zipok := request.URL.Query()["zip"]
	date, dateok := request.URL.Query()["date"]

	zipvar := "0000"
	datevar := "2022-07-02"

	if zipok && dateok {
		zipvar = zip[0]
		datevar = date[0]
	}

	const (
		layoutISO = "2006-01-02"
	)

	date_formatted, _ := time.Parse(layoutISO, datevar)

	cntx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	//Validating zipcode
	projection := bson.D{
		{"_id", 1},
	}

	cursor_chef, err := connection.chef.Find(cntx, bson.D{{"zipcode", zipvar}}, options.Find().SetProjection(projection))
	if err != nil {
		panic(err)
	}

	var chefid = []bson.M{}
	if err = cursor_chef.All(cntx, &chefid); err != nil {
		panic(err)
	}
	for _, result := range chefid {
		fmt.Println(result)
	}

	chefid_array := make([]int32, len(chefid))
	for i, doc := range chefid {
		chefid_array[i] = doc["_id"].(int32)
	}
	fmt.Println(date_formatted)
	fmt.Println(chefid_array)

	//Date

	cursor_menu, err := connection.menu.Find(cntx, bson.M{
		"datefrom": bson.M{"$lt": date_formatted},
		"dateto":   bson.M{"$gt": date_formatted},
		"chefID":   bson.M{"$in": chefid_array},
	})

	if err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}

	if err = cursor_menu.All(cntx, &menus); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}

	for _, menu := range menus {
		fmt.Println(menu)
	}
	json.NewEncoder(response).Encode(menus)
}

func (connection Connection) CreateOrderEndpoint(response http.ResponseWriter, request *http.Request) {
	response.Header().Add("content-type", "application/json")
	var order_insert Order
	json.NewDecoder(request.Body).Decode(&order_insert)

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	const (
		layoutISO = "2006-01-02"
	)

	test_date, _ := time.Parse(layoutISO, order_insert.Order_date)

	//Validating zipcode
	projection := bson.D{
		{"_id", 1},
	}

	cursor_chef, err := connection.chef.Find(ctx, bson.D{{"zipcode", order_insert.Zip}}, options.Find().SetProjection(projection))
	if err != nil {
		panic(err)
	}

	var chefid = []bson.M{}
	if err = cursor_chef.All(ctx, &chefid); err != nil {
		panic(err)
	}

	chefid_array := make([]int32, len(chefid))
	for i, doc := range chefid {
		chefid_array[i] = doc["_id"].(int32)
	}

	//Menu id
	id_int, _ := strconv.ParseInt(order_insert.MenuID, 0, 32)

	//fmt.Println(order_insert.Ordered_items[0].Item_name)

	item_name_array := []string{}
	item_quantity_array := []int32{}

	for _, v := range order_insert.Ordered_items {
		item_name_array = append(item_name_array, v.Item_name)
		item_quantity_array = append(item_quantity_array, v.Item_quantity)

	}

	cursor_aggregate_menu, err := connection.menu.Aggregate(ctx,
		[]bson.M{
			bson.M{"$unwind": "$items"},
			bson.M{
				"$project": bson.M{
					"item_name": "$items.item_name",
					"item_cost": "$items.item_cost",
				},
			},
		},
	)

	var items_info = []bson.M{}
	if err = cursor_aggregate_menu.All(ctx, &items_info); err != nil {
		panic(err)
	}

	var all_items_cost int32
	all_items_cost = 0

	for index1, i := range item_name_array {
		for _, result := range items_info {
			if i == result["item_name"] {
				var cost int32 = int32(result["item_cost"].(int32))
				all_items_cost = all_items_cost + item_quantity_array[index1]*cost
			}
		}
	}

	cursor_order, err := connection.menu.Find(ctx, bson.M{
		"_id":      bson.M{"$eq": id_int},
		"datefrom": bson.M{"$lt": test_date},
		"dateto":   bson.M{"$gt": test_date},
		"chefID":   bson.M{"$in": chefid_array},
	})
	var odr []Order
	if err = cursor_order.All(ctx, &odr); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		response.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}

	if odr == nil {
		fmt.Fprintf(response, "Invalid request")
		fmt.Println("Invalid request")
		return
	}

	order_insert.Total_cost = all_items_cost
	result, _ := connection.order.InsertOne(ctx, order_insert)
	fmt.Fprintf(response, "Successfully added order...with orderID")
	fmt.Println("Successfully added order")
	json.NewEncoder(response).Encode(result)
}

func main() {
	fmt.Println("Starting the application...")

	cntx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	client, err := mongo.Connect(cntx, options.Client().ApplyURI("mongodb+srv://abhi360g:SAXX24aibh34@gotest1.kzj5n.mongodb.net/goTest1?retryWrites=true&w=majority"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(cntx)

	connection := Connection{
		chef:  client.Database("hungry").Collection("chef"),
		menu:  client.Database("hungry").Collection("menu"),
		order: client.Database("hungry").Collection("order"),
		user:  client.Database("hungry").Collection("user"),
	}

	router := mux.NewRouter()
	router.HandleFunc("/menus", connection.GetMenusEndpoint).Methods("GET")
	router.HandleFunc("/order", connection.CreateOrderEndpoint).Methods("POST")
	http.ListenAndServe(":8000", router)
}
