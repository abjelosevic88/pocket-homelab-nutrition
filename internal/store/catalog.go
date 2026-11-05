package store

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// CatalogFood is one food of the built-in list: typical values per 100 g
// (or 100 ml for drinks), from food composition tables, rounded. A person's
// copy is made the first time they log it, so they can correct it.
type CatalogFood struct {
	Key      string
	Name     string
	Category string
	Unit     string
	Nutrients
	Servings []Serving
	Aliases  []string
}

func f64(v float64) *float64 { return &v }

// cf builds a catalog food: kcal, protein, carbs, fat, fibre, sugar,
// saturated fat (g) and sodium (mg) per 100, then servings as "label=amount".
func cf(key, name, cat, unit string, kcal, p, c, fat, fib, sug, sat, na float64, servings ...string) CatalogFood {
	out := CatalogFood{Key: key, Name: name, Category: cat, Unit: unit,
		Nutrients: Nutrients{Kcal: kcal, Protein: p, Carbs: c, Fat: fat, Fiber: f64(fib), Sugar: f64(sug), SatFat: f64(sat), Sodium: f64(na)}}
	for _, s := range servings {
		if strings.HasPrefix(s, "aka:") {
			out.Aliases = append(out.Aliases, strings.Split(strings.TrimPrefix(s, "aka:"), "|")...)
			continue
		}
		i := strings.LastIndex(s, "=")
		v, _ := strconv.ParseFloat(s[i+1:], 64)
		out.Servings = append(out.Servings, Serving{Label: s[:i], Amount: v})
	}
	return out
}

// Catalog is the built-in food list.
var Catalog = []CatalogFood{
	// fruit
	cf("apple", "Apple", "fruit", "g", 52, 0.3, 13.8, 0.2, 2.4, 10.4, 0, 1, "1 medium apple=182", "aka:jabuka"),
	cf("banana", "Banana", "fruit", "g", 89, 1.1, 22.8, 0.3, 2.6, 12.2, 0.1, 1, "1 medium banana=118", "aka:banana"),
	cf("orange", "Orange", "fruit", "g", 47, 0.9, 11.8, 0.1, 2.4, 9.4, 0, 0, "1 orange=131", "aka:narandža|narandza"),
	cf("mandarin", "Mandarin", "fruit", "g", 53, 0.8, 13.3, 0.3, 1.8, 10.6, 0, 2, "1 mandarin=88", "aka:clementine|tangerine|mandarina"),
	cf("pear", "Pear", "fruit", "g", 57, 0.4, 15.2, 0.1, 3.1, 9.8, 0, 1, "1 pear=178", "aka:kruška|kruska"),
	cf("peach", "Peach", "fruit", "g", 39, 0.9, 9.5, 0.3, 1.5, 8.4, 0, 0, "1 peach=150", "aka:breskva"),
	cf("plum", "Plums", "fruit", "g", 46, 0.7, 11.4, 0.3, 1.4, 9.9, 0, 0, "1 plum=66", "aka:šljiva|sljiva"),
	cf("grapes", "Grapes", "fruit", "g", 69, 0.7, 18.1, 0.2, 0.9, 15.5, 0.1, 2, "1 cup=151", "aka:grožđe|grozdje"),
	cf("strawberries", "Strawberries", "fruit", "g", 32, 0.7, 7.7, 0.3, 2, 4.9, 0, 1, "1 cup=152", "aka:jagode"),
	cf("blueberries", "Blueberries", "fruit", "g", 57, 0.7, 14.5, 0.3, 2.4, 10, 0, 1, "1 cup=148", "aka:borovnice"),
	cf("raspberries", "Raspberries", "fruit", "g", 52, 1.2, 11.9, 0.7, 6.5, 4.4, 0, 1, "1 cup=123", "aka:maline"),
	cf("watermelon", "Watermelon", "fruit", "g", 30, 0.6, 7.6, 0.2, 0.4, 6.2, 0, 1, "1 slice=280", "aka:lubenica"),
	cf("kiwi", "Kiwi", "fruit", "g", 61, 1.1, 14.7, 0.5, 3, 9, 0, 3, "1 kiwi=69"),
	cf("pineapple", "Pineapple", "fruit", "g", 50, 0.5, 13.1, 0.1, 1.4, 9.9, 0, 1, "1 cup=165", "aka:ananas"),
	cf("mango", "Mango", "fruit", "g", 60, 0.8, 15, 0.4, 1.6, 13.7, 0.1, 1, "1 cup=165"),
	cf("avocado", "Avocado", "fruit", "g", 160, 2, 8.5, 14.7, 6.7, 0.7, 2.1, 7, "half an avocado=75", "1 avocado=150"),
	cf("raisins", "Raisins", "fruit", "g", 299, 3.1, 79.2, 0.5, 3.7, 59.2, 0.1, 11, "a handful=30", "aka:grožđice|suho grožđe"),
	cf("dates", "Dates, dried", "fruit", "g", 282, 2.5, 75, 0.4, 8, 63, 0, 2, "1 date=8", "1 medjool date=24", "aka:hurme"),
	// vegetables
	cf("tomato", "Tomato", "vegetables", "g", 18, 0.9, 3.9, 0.2, 1.2, 2.6, 0, 5, "1 tomato=123", "aka:paradajz"),
	cf("cucumber", "Cucumber", "vegetables", "g", 15, 0.7, 3.6, 0.1, 0.5, 1.7, 0, 2, "1 cucumber=300", "aka:krastavac"),
	cf("carrot", "Carrot", "vegetables", "g", 41, 0.9, 9.6, 0.2, 2.8, 4.7, 0, 69, "1 carrot=61", "aka:mrkva"),
	cf("broccoli", "Broccoli", "vegetables", "g", 34, 2.8, 6.6, 0.4, 2.6, 1.7, 0.1, 33, "1 cup=91", "aka:brokula"),
	cf("spinach", "Spinach", "vegetables", "g", 23, 2.9, 3.6, 0.4, 2.2, 0.4, 0.1, 79, "1 cup raw=30", "aka:špinat|spinat"),
	cf("lettuce", "Lettuce, green salad", "vegetables", "g", 15, 1.4, 2.9, 0.2, 1.3, 0.8, 0, 28, "1 bowl=70", "aka:salata|zelena salata"),
	cf("bell-pepper", "Bell pepper", "vegetables", "g", 31, 1, 6, 0.3, 2.1, 4.2, 0, 4, "1 pepper=119", "aka:paprika|capsicum"),
	cf("onion", "Onion", "vegetables", "g", 40, 1.1, 9.3, 0.1, 1.7, 4.2, 0, 4, "1 onion=110", "aka:luk|crveni luk"),
	cf("cabbage", "Cabbage", "vegetables", "g", 25, 1.3, 5.8, 0.1, 2.5, 3.2, 0, 18, "1 cup=89", "aka:kupus"),
	cf("mushrooms", "Mushrooms", "vegetables", "g", 22, 3.1, 3.3, 0.3, 1, 2, 0, 5, "1 cup=70", "aka:gljive|šampinjoni"),
	cf("zucchini", "Courgette (zucchini)", "vegetables", "g", 17, 1.2, 3.1, 0.3, 1, 2.5, 0.1, 8, "1 courgette=200", "aka:zucchini|tikvica"),
	cf("green-beans", "Green beans", "vegetables", "g", 31, 1.8, 7, 0.2, 2.7, 3.3, 0, 6, "1 cup=110", "aka:mahune|boranija"),
	cf("peas", "Peas", "vegetables", "g", 81, 5.4, 14.5, 0.4, 5.1, 5.7, 0.1, 5, "1 cup=145", "aka:grašak|grasak"),
	cf("sweetcorn", "Sweetcorn", "vegetables", "g", 86, 3.3, 19, 1.4, 2.7, 6.3, 0.3, 15, "1 cob=90", "1 cup=145", "aka:corn|kukuruz"),
	cf("potato-boiled", "Potatoes, boiled", "vegetables", "g", 87, 1.9, 20.1, 0.1, 1.8, 0.9, 0, 4, "1 medium potato=170", "aka:potato|krompir"),
	cf("potato-mashed", "Mashed potatoes", "vegetables", "g", 113, 1.9, 15.7, 4.6, 1.5, 1.4, 1.1, 300, "1 cup=210", "aka:pire|pire krompir"),
	cf("sweet-potato", "Sweet potato, baked", "vegetables", "g", 90, 2, 20.7, 0.2, 3.3, 6.5, 0, 36, "1 medium=130", "aka:batat"),
	cf("fries", "French fries", "vegetables", "g", 312, 3.4, 41, 15, 3.8, 0.3, 2.3, 210, "a medium portion=117", "a large portion=154", "aka:chips|pomfrit"),
	// grains
	cf("rice-white", "Rice, white, cooked", "grains", "g", 130, 2.7, 28.2, 0.3, 0.4, 0.1, 0.1, 1, "1 cup=158", "aka:rice|riža|riza"),
	cf("rice-brown", "Rice, brown, cooked", "grains", "g", 112, 2.3, 23.5, 0.8, 1.8, 0.4, 0.2, 5, "1 cup=195"),
	cf("pasta-cooked", "Pasta, cooked", "grains", "g", 158, 5.8, 30.9, 0.9, 1.8, 0.6, 0.2, 1, "1 cup=140", "a plate=250", "aka:spaghetti|tjestenina|makaroni|penne"),
	cf("pasta-dry", "Pasta, dry", "grains", "g", 371, 13, 74.7, 1.5, 3.2, 2.7, 0.3, 6, "a portion=80"),
	cf("oats", "Oats, rolled", "grains", "g", 379, 13.2, 67.7, 6.5, 10.1, 1, 1.1, 6, "half a cup=40", "aka:porridge oats|zobene pahuljice|ovsene pahuljice"),
	cf("porridge", "Porridge with milk", "grains", "g", 98, 4.2, 13, 3.2, 1.5, 4.5, 1.6, 40, "1 bowl=250", "aka:oatmeal|zobena kaša"),
	cf("cornflakes", "Cornflakes", "grains", "g", 357, 7.5, 84, 0.4, 3.3, 9.5, 0.1, 729, "1 bowl=30"),
	cf("muesli", "Muesli", "grains", "g", 360, 9.7, 66, 5.9, 7.3, 21, 1, 50, "1 bowl=50", "aka:musli"),
	cf("granola", "Granola", "grains", "g", 471, 10.5, 64, 20, 7, 24, 3.8, 26, "1 bowl=50"),
	cf("quinoa", "Quinoa, cooked", "grains", "g", 120, 4.4, 21.3, 1.9, 2.8, 0.9, 0.2, 7, "1 cup=185"),
	cf("couscous", "Couscous, cooked", "grains", "g", 112, 3.8, 23.2, 0.2, 1.4, 0.1, 0, 5, "1 cup=157", "aka:kus kus"),
	cf("bulgur", "Bulgur, cooked", "grains", "g", 83, 3.1, 18.6, 0.2, 4.5, 0.1, 0, 5, "1 cup=182"),
	// bread
	cf("bread-white", "White bread", "bread", "g", 266, 7.6, 49.4, 3.3, 2.7, 5.7, 0.7, 490, "1 slice=28", "aka:bread|toast|hljeb|kruh|bijeli hljeb"),
	cf("bread-wholemeal", "Wholemeal bread", "bread", "g", 252, 12.4, 42.7, 3.5, 6, 4.4, 0.7, 450, "1 slice=32", "aka:whole wheat bread|integralni hljeb|crni hljeb"),
	cf("bread-rye", "Rye bread", "bread", "g", 259, 8.5, 48.3, 3.3, 5.8, 3.9, 0.6, 600, "1 slice=32", "aka:raženi hljeb"),
	cf("baguette", "Baguette", "bread", "g", 270, 9, 55, 1.5, 2.5, 3, 0.3, 600, "a 10 cm piece=60", "aka:french bread"),
	cf("somun", "Somun (flatbread)", "bread", "g", 265, 8.5, 53, 1.5, 2.5, 2, 0.3, 500, "half a somun=110", "1 somun=220", "aka:lepinja|pita bread"),
	cf("pita", "Pita bread", "bread", "g", 275, 9.1, 55.7, 1.2, 2.2, 1.3, 0.2, 536, "1 pita=60"),
	cf("tortilla", "Tortilla wrap", "bread", "g", 310, 8.3, 51.6, 7.7, 3.5, 2.3, 2.9, 600, "1 tortilla=60", "aka:wrap"),
	cf("bagel", "Bagel", "bread", "g", 250, 10, 49, 1.5, 2.1, 6, 0.2, 439, "1 bagel=105"),
	cf("croissant", "Croissant", "bread", "g", 406, 8.2, 45.8, 21, 2.6, 11.3, 11.7, 470, "1 croissant=57", "aka:kroasan"),
	cf("rice-cake", "Rice cakes", "bread", "g", 387, 8.2, 81.5, 2.8, 4.2, 0.9, 0.6, 29, "1 rice cake=9", "aka:rižini krekeri|galeta"),
	// dairy
	cf("milk-whole", "Milk, whole (3.5%)", "dairy", "ml", 64, 3.3, 4.8, 3.5, 0, 4.8, 2.2, 43, "1 glass=250", "a splash=30", "aka:milk|mlijeko|full fat milk"),
	cf("milk-semi", "Milk, semi-skimmed (1.5%)", "dairy", "ml", 46, 3.4, 4.9, 1.5, 0, 4.9, 1, 44, "1 glass=250", "a splash=30", "aka:low fat milk|2% milk"),
	cf("milk-skim", "Milk, skimmed", "dairy", "ml", 34, 3.4, 5, 0.1, 0, 5, 0.1, 42, "1 glass=250"),
	cf("yoghurt-drink", "Drinking yoghurt (2.8%)", "dairy", "ml", 56, 3.2, 4.2, 2.8, 0, 4.2, 1.8, 45, "1 glass=200", "1 bottle=500", "aka:jogurt|tekući jogurt"),
	cf("kefir", "Kefir", "dairy", "ml", 52, 3.4, 4.5, 2, 0, 4.5, 1.3, 40, "1 glass=250"),
	cf("yoghurt-plain", "Yoghurt, plain (3%)", "dairy", "g", 61, 3.5, 4.7, 3.3, 0, 4.7, 2.1, 46, "1 pot=180", "aka:yogurt|natural yoghurt"),
	cf("greek-yoghurt", "Greek yoghurt (2%)", "dairy", "g", 73, 9.9, 3.9, 1.9, 0, 3.6, 1.2, 34, "1 pot=170", "aka:greek yogurt|grčki jogurt"),
	cf("greek-yoghurt-full", "Greek yoghurt, full fat", "dairy", "g", 97, 9, 4, 5, 0, 4, 2.4, 35, "1 pot=170"),
	cf("skyr", "Skyr", "dairy", "g", 63, 11, 4, 0.2, 0, 4, 0.1, 40, "1 pot=150"),
	cf("cottage-cheese", "Cottage cheese", "dairy", "g", 98, 11.1, 3.4, 4.3, 0, 2.7, 1.7, 364, "half a cup=113", "aka:posni sir|svježi sir"),
	cf("cheddar", "Cheddar", "dairy", "g", 403, 24.9, 1.3, 33.1, 0, 0.5, 21, 621, "1 slice=28", "aka:cheese"),
	cf("gouda", "Gouda", "dairy", "g", 356, 24.9, 2.2, 27.4, 0, 2.2, 17.6, 819, "1 slice=20", "aka:trapist|edamer|hard cheese"),
	cf("mozzarella", "Mozzarella", "dairy", "g", 280, 22, 2.2, 20, 0, 1, 12, 500, "1 ball=125", "a slice=20"),
	cf("feta", "Feta", "dairy", "g", 264, 14.2, 4.1, 21.3, 0, 4.1, 14.9, 1116, "a portion=30", "aka:bijeli sir|white cheese|sir"),
	cf("parmesan", "Parmesan", "dairy", "g", 392, 35.8, 3.2, 25.8, 0, 0.8, 16.4, 1376, "1 tbsp grated=5", "aka:parmezan"),
	cf("cream-cheese", "Cream cheese", "dairy", "g", 342, 5.9, 4.1, 34.2, 0, 3.2, 19.3, 321, "1 tbsp=15", "aka:krem sir"),
	cf("kajmak", "Kajmak", "dairy", "g", 550, 4, 2.5, 58, 0, 2.5, 37, 300, "1 tbsp=20", "aka:clotted cream"),
	cf("sour-cream", "Sour cream", "dairy", "g", 193, 2.1, 4.6, 19, 0, 3.4, 10, 31, "1 tbsp=15", "aka:pavlaka|vrhnje"),
	cf("butter", "Butter", "dairy", "g", 717, 0.9, 0.1, 81, 0, 0.1, 51, 11, "1 tsp=5", "1 tbsp=14", "aka:maslac|puter"),
	cf("ice-cream", "Ice cream, vanilla", "dairy", "g", 207, 3.5, 23.6, 11, 0.7, 21.2, 6.8, 80, "1 scoop=66", "aka:sladoled"),
	// eggs
	cf("egg", "Egg", "eggs", "g", 143, 12.6, 0.7, 9.5, 0, 0.4, 3.1, 142, "1 egg=50", "1 large egg=60", "aka:eggs|boiled egg|jaje|jaja"),
	cf("egg-fried", "Fried egg", "eggs", "g", 196, 13.6, 0.8, 14.8, 0, 0.4, 4.3, 207, "1 egg=46", "aka:jaje na oko"),
	cf("eggs-scrambled", "Scrambled eggs", "eggs", "g", 149, 10, 1.6, 11, 0, 1.4, 3.3, 145, "from 2 eggs=120", "aka:omelette|omlet|kajgana"),
	cf("egg-white", "Egg white", "eggs", "g", 52, 10.9, 0.7, 0.2, 0, 0.7, 0, 166, "1 egg white=33", "aka:bjelanjak"),
	// meat
	cf("chicken-breast", "Chicken breast, cooked", "meat", "g", 165, 31, 0, 3.6, 0, 0, 1, 74, "1 fillet=150", "aka:chicken|piletina|pileća prsa|pileci file"),
	cf("chicken-breast-raw", "Chicken breast, raw", "meat", "g", 120, 22.5, 0, 2.6, 0, 0, 0.6, 45, "1 fillet=175"),
	cf("chicken-thigh", "Chicken thigh, cooked", "meat", "g", 179, 24.8, 0, 8.2, 0, 0, 2.3, 106, "1 thigh=100", "aka:batak|karabatak"),
	cf("turkey-breast", "Turkey breast, cooked", "meat", "g", 135, 30, 0, 1, 0, 0, 0.3, 55, "a portion=120", "aka:ćuretina|puretina"),
	cf("beef-mince-10", "Beef mince, 10% fat, raw", "meat", "g", 176, 20, 0, 10, 0, 0, 4, 66, "a portion=125", "aka:ground beef|mljeveno meso|faširano"),
	cf("beef-mince-20", "Beef mince, 20% fat, raw", "meat", "g", 254, 17.2, 0, 20, 0, 0, 7.6, 66, "a portion=125"),
	cf("steak", "Beef steak, cooked", "meat", "g", 205, 28, 0, 10, 0, 0, 4, 60, "1 steak=200", "aka:sirloin|biftek|govedina|beef"),
	cf("pork-chop", "Pork chop, cooked", "meat", "g", 231, 25.7, 0, 13.5, 0, 0, 4.7, 62, "1 chop=150", "aka:svinjetina|kotlet"),
	cf("lamb", "Lamb, roast", "meat", "g", 294, 25.6, 0, 20.9, 0, 0, 8.8, 72, "a portion=150", "aka:janjetina"),
	cf("cevapi", "Ćevapi", "meat", "g", 260, 18, 3, 20, 0, 0.5, 8, 700, "1 ćevap=30", "5 ćevapi=150", "10 ćevapi=300", "aka:cevapi|cevapcici|ćevapčići"),
	cf("pljeskavica", "Pljeskavica", "meat", "g", 250, 17, 4, 18, 0, 1, 7, 700, "1 pljeskavica=200", "aka:burger patty"),
	cf("bacon", "Bacon, cooked", "meat", "g", 541, 37, 1.4, 42, 0, 0, 14, 1717, "1 rasher=8", "aka:slanina"),
	cf("ham", "Ham, sliced", "meat", "g", 120, 18, 1.5, 4.5, 0, 1, 1.5, 1100, "1 slice=15", "aka:šunka|sunka"),
	cf("salami", "Salami", "meat", "g", 380, 22, 2, 31, 0, 1, 11, 1700, "1 slice=10", "aka:kobasica|sudžuk"),
	cf("sausage", "Sausage, cooked", "meat", "g", 300, 13, 2, 26, 0, 1, 9, 750, "1 sausage=75", "aka:hot dog|viršla|hrenovka"),
	// fish
	cf("salmon", "Salmon, cooked", "fish", "g", 206, 22.1, 0, 12.4, 0, 0, 2.5, 61, "1 fillet=150", "aka:losos"),
	cf("tuna-water", "Tuna in water, drained", "fish", "g", 116, 25.5, 0, 0.8, 0, 0, 0.2, 247, "1 can=120", "aka:tuna|tunjevina"),
	cf("tuna-oil", "Tuna in oil, drained", "fish", "g", 198, 29.1, 0, 8.2, 0, 0, 1.5, 354, "1 can=120"),
	cf("cod", "Cod, cooked", "fish", "g", 105, 22.8, 0, 0.9, 0, 0, 0.2, 78, "1 fillet=150", "aka:white fish|oslić|bakalar"),
	cf("trout", "Trout, cooked", "fish", "g", 168, 23.8, 0, 7.4, 0, 0, 2.2, 61, "1 fillet=150", "aka:pastrmka|pastrva"),
	cf("sardines", "Sardines in oil", "fish", "g", 208, 24.6, 0, 11.5, 0, 0, 1.5, 307, "1 can=90", "aka:srdele|sardine"),
	cf("shrimp", "Prawns, cooked", "fish", "g", 99, 24, 0.2, 0.3, 0, 0, 0.1, 111, "a portion=100", "aka:shrimp|škampi|kozice"),
	cf("fish-fingers", "Fish fingers", "fish", "g", 220, 12, 20, 10, 1, 1.5, 1.2, 380, "1 fish finger=28", "aka:fish sticks|riblji štapići"),
	// legumes
	cf("chickpeas", "Chickpeas, cooked", "legumes", "g", 164, 8.9, 27.4, 2.6, 7.6, 4.8, 0.3, 7, "1 cup=164", "aka:slanutak|leblebija"),
	cf("lentils", "Lentils, cooked", "legumes", "g", 116, 9, 20.1, 0.4, 7.9, 1.8, 0.1, 2, "1 cup=198", "aka:sočivo|leća"),
	cf("kidney-beans", "Kidney beans, cooked", "legumes", "g", 127, 8.7, 22.8, 0.5, 6.4, 0.3, 0.1, 2, "1 cup=177", "aka:beans|grah"),
	cf("black-beans", "Black beans, cooked", "legumes", "g", 132, 8.9, 23.7, 0.5, 8.7, 0.3, 0.1, 1, "1 cup=172"),
	cf("baked-beans", "Baked beans", "legumes", "g", 84, 4.8, 15, 0.4, 4, 5, 0.1, 380, "half a can=200"),
	cf("tofu", "Tofu, firm", "legumes", "g", 144, 17.3, 2.8, 8.7, 2.3, 0.6, 1.3, 14, "a portion=100"),
	cf("hummus", "Hummus", "legumes", "g", 166, 7.9, 14.3, 9.6, 6, 0.3, 1.4, 379, "1 tbsp=15", "a portion=50", "aka:humus"),
	// nuts and seeds
	cf("almonds", "Almonds", "nuts", "g", 579, 21.2, 21.6, 49.9, 12.5, 4.4, 3.8, 1, "a handful=30", "aka:bademi"),
	cf("walnuts", "Walnuts", "nuts", "g", 654, 15.2, 13.7, 65.2, 6.7, 2.6, 6.1, 2, "a handful=30", "aka:orasi"),
	cf("hazelnuts", "Hazelnuts", "nuts", "g", 628, 15, 16.7, 60.8, 9.7, 4.3, 4.5, 0, "a handful=30", "aka:lješnjaci|ljesnjaci"),
	cf("peanuts", "Peanuts", "nuts", "g", 567, 25.8, 16.1, 49.2, 8.5, 4.7, 6.3, 18, "a handful=30", "aka:kikiriki"),
	cf("cashews", "Cashews", "nuts", "g", 553, 18.2, 30.2, 43.9, 3.3, 5.9, 7.8, 12, "a handful=30", "aka:indijski orah"),
	cf("peanut-butter", "Peanut butter", "nuts", "g", 588, 25.1, 19.6, 50.4, 6, 9.2, 10.3, 400, "1 tbsp=16", "aka:maslac od kikirikija"),
	cf("chia", "Chia seeds", "nuts", "g", 486, 16.5, 42.1, 30.7, 34.4, 0, 3.3, 16, "1 tbsp=12"),
	cf("sunflower-seeds", "Sunflower seeds", "nuts", "g", 584, 20.8, 20, 51.5, 8.6, 2.6, 4.5, 9, "a handful=30", "aka:sjemenke suncokreta|koštice"),
	cf("pumpkin-seeds", "Pumpkin seeds", "nuts", "g", 559, 30.2, 10.7, 49, 6, 1.4, 8.7, 7, "a handful=30", "aka:bučine koštice"),
	// fats, spreads and sauces
	cf("olive-oil", "Olive oil", "fats", "g", 884, 0, 0, 100, 0, 0, 13.8, 2, "1 tsp=4.5", "1 tbsp=14", "aka:maslinovo ulje|oil"),
	cf("sunflower-oil", "Sunflower oil", "fats", "g", 884, 0, 0, 100, 0, 0, 10.3, 0, "1 tsp=4.5", "1 tbsp=14", "aka:ulje|vegetable oil"),
	cf("mayonnaise", "Mayonnaise", "fats", "g", 680, 1, 0.6, 75, 0, 0.6, 11.7, 635, "1 tbsp=14", "aka:majoneza|mayo"),
	cf("ketchup", "Ketchup", "fats", "g", 101, 1, 27.4, 0.1, 0.3, 22.8, 0, 907, "1 tbsp=17", "aka:kečap"),
	cf("mustard", "Mustard", "fats", "g", 60, 3.7, 5.8, 3.3, 4, 0.9, 0.2, 1100, "1 tsp=5", "aka:senf"),
	cf("ajvar", "Ajvar", "fats", "g", 110, 1.5, 8, 8, 2.5, 6, 1, 600, "1 tbsp=20"),
	cf("pesto", "Pesto", "fats", "g", 450, 5, 4, 45, 1.5, 1, 7, 1000, "1 tbsp=15"),
	cf("honey", "Honey", "sweets", "g", 304, 0.3, 82.4, 0, 0.2, 82.1, 0, 4, "1 tsp=7", "1 tbsp=21", "aka:med"),
	cf("jam", "Jam", "sweets", "g", 278, 0.4, 69, 0.1, 1.1, 48.5, 0, 32, "1 tbsp=20", "aka:džem|pekmez|marmelada"),
	cf("sugar", "Sugar", "sweets", "g", 387, 0, 100, 0, 0, 100, 0, 1, "1 tsp=4", "1 cube=3", "aka:šećer|secer"),
	cf("nutella", "Hazelnut chocolate spread", "sweets", "g", 539, 6.3, 57.5, 30.9, 3.4, 56.3, 10.6, 41, "1 tbsp=19", "aka:nutella|eurokrem|krem"),
	// sweets and snacks
	cf("chocolate-dark", "Dark chocolate (70%)", "sweets", "g", 598, 7.8, 45.9, 42.6, 10.9, 24, 24.5, 20, "1 square=10", "1 bar=100", "aka:čokolada|cokolada"),
	cf("chocolate-milk", "Milk chocolate", "sweets", "g", 535, 7.7, 59.4, 29.7, 3.4, 51.5, 18.5, 79, "1 row=25", "1 bar=100", "aka:chocolate"),
	cf("cookies", "Biscuits, chocolate chip", "sweets", "g", 488, 5.4, 64, 24, 2, 33, 7.5, 333, "1 biscuit=16", "aka:cookies|keks|keksi"),
	cf("cake-chocolate", "Chocolate cake", "sweets", "g", 371, 5.3, 53, 16, 2.2, 36, 5, 334, "1 slice=95", "aka:cake|torta|kolač"),
	cf("baklava", "Baklava", "sweets", "g", 428, 6.7, 37.6, 29, 2.5, 24, 9, 300, "1 piece=60", "aka:baklave|tufahija|hurmašice"),
	cf("doughnut", "Doughnut, glazed", "sweets", "g", 421, 5.7, 51, 22.5, 1.5, 23, 10.5, 316, "1 doughnut=60", "aka:donut|krofna"),
	cf("muffin", "Muffin", "sweets", "g", 377, 4.4, 51, 17, 1.6, 26, 3, 300, "1 muffin=110"),
	cf("gummies", "Gummy sweets", "sweets", "g", 343, 6.9, 77, 0.2, 0, 46, 0, 40, "a handful=25", "aka:gummy bears|bombone|haribo"),
	cf("crisps", "Crisps", "snacks", "g", 536, 7, 53, 34.6, 4.4, 0.3, 3.1, 525, "1 small bag=30", "aka:potato chips|čips|cips"),
	cf("popcorn", "Popcorn", "snacks", "g", 387, 12.9, 77.8, 4.5, 14.5, 0.9, 0.6, 8, "1 bowl=25", "aka:kokice"),
	cf("pretzels", "Pretzels", "snacks", "g", 380, 10.3, 79.8, 2.6, 2.8, 2.8, 0.4, 1300, "a handful=30", "aka:perece|štapići|smoki"),
	cf("protein-bar", "Protein bar", "snacks", "g", 350, 30, 35, 10, 8, 5, 4, 200, "1 bar=60"),
	cf("cereal-bar", "Cereal bar", "snacks", "g", 400, 6, 65, 13, 5, 30, 3, 100, "1 bar=25", "aka:muesli bar|granola bar"),
	cf("whey", "Whey protein powder", "snacks", "g", 400, 78, 8, 6, 0, 6, 3, 300, "1 scoop=30", "aka:protein shake|protein powder|whey"),
	// drinks
	cf("coffee", "Coffee, black", "drinks", "ml", 1, 0.1, 0, 0, 0, 0, 0, 2, "1 cup=200", "1 espresso=30", "1 fildžan=60", "aka:espresso|americano|bosanska kafa|kafa|kava"),
	cf("cappuccino", "Cappuccino", "drinks", "ml", 40, 2.2, 3.3, 2, 0, 3.3, 1.2, 35, "1 cup=180", "aka:kapućino|kapucino|flat white"),
	cf("latte", "Caffè latte", "drinks", "ml", 54, 3, 4.5, 2.7, 0, 4.5, 1.6, 40, "1 cup=240", "aka:latte|kafa s mlijekom|macchiato"),
	cf("tea", "Tea, no sugar", "drinks", "ml", 1, 0, 0.3, 0, 0, 0, 0, 3, "1 cup=250", "aka:čaj|caj|green tea|herbal tea"),
	cf("hot-chocolate", "Hot chocolate", "drinks", "ml", 77, 3.5, 10.7, 2.3, 1, 9.7, 1.4, 44, "1 cup=250", "aka:topla čokolada|kakao"),
	cf("orange-juice", "Orange juice", "drinks", "ml", 45, 0.7, 10.4, 0.2, 0.2, 8.4, 0, 1, "1 glass=250", "aka:juice|sok od narandže|džus"),
	cf("apple-juice", "Apple juice", "drinks", "ml", 46, 0.1, 11.3, 0.1, 0.2, 9.6, 0, 4, "1 glass=250", "aka:sok od jabuke"),
	cf("cola", "Cola", "drinks", "ml", 42, 0, 10.6, 0, 0, 10.6, 0, 4, "1 can=330", "1 glass=250", "aka:coke|coca-cola|pepsi|soft drink|sok"),
	cf("cola-zero", "Cola, sugar free", "drinks", "ml", 0.3, 0, 0, 0, 0, 0, 0, 10, "1 can=330", "aka:diet coke|coke zero|pepsi max"),
	cf("energy-drink", "Energy drink", "drinks", "ml", 45, 0, 11, 0, 0, 11, 0, 80, "1 can=250", "aka:red bull|monster"),
	cf("beer", "Beer, lager", "drinks", "ml", 43, 0.5, 3.6, 0, 0, 0, 0, 4, "1 bottle (0.5 l)=500", "1 can (0.33 l)=330", "aka:pivo|lager"),
	cf("wine-red", "Red wine", "drinks", "ml", 85, 0.1, 2.6, 0, 0, 0.6, 0, 4, "1 glass=150", "aka:wine|crno vino|vino"),
	cf("wine-white", "White wine", "drinks", "ml", 82, 0.1, 2.6, 0, 0, 1, 0, 5, "1 glass=150", "aka:bijelo vino"),
	cf("spirits", "Spirits (40%)", "drinks", "ml", 220, 0, 0, 0, 0, 0, 0, 1, "1 shot=30", "1 double=50", "aka:rakija|vodka|whisky|gin|šljivovica|loza"),
	// dishes
	cf("pizza", "Pizza margherita", "meals", "g", 250, 11, 30, 9.5, 2, 3.5, 4.5, 560, "1 slice=100", "a whole pizza=450", "aka:pizza"),
	cf("burek", "Burek (meat pie)", "meals", "g", 300, 10, 23, 19, 1.2, 1, 7, 500, "a portion=250", "aka:pita|burek s mesom"),
	cf("sirnica", "Sirnica (cheese pie)", "meals", "g", 290, 10, 24, 17, 1, 1.5, 7.5, 600, "a portion=250", "aka:pita sa sirom|cheese pie"),
	cf("zeljanica", "Zeljanica (spinach pie)", "meals", "g", 220, 7, 22, 12, 2, 1, 5, 450, "a portion=250", "aka:spinach pie"),
	cf("sarma", "Sarma (cabbage rolls)", "meals", "g", 125, 6.5, 9, 7, 1.8, 1.5, 2.6, 450, "1 sarma=120"),
	cf("grah", "Bean stew", "meals", "g", 95, 5.5, 12, 2.8, 4, 1.5, 0.9, 300, "a plate=350", "aka:grah|bean soup"),
	cf("chicken-soup", "Chicken soup", "meals", "ml", 36, 3, 3.5, 1.2, 0.3, 0.4, 0.3, 340, "1 bowl=300", "aka:soup|supa|čorba"),
	cf("lasagne", "Lasagne", "meals", "g", 150, 8.5, 14, 7, 1.5, 3.5, 3.5, 350, "a portion=300", "aka:lasagna"),
	cf("bolognese", "Spaghetti bolognese", "meals", "g", 135, 7, 17, 4.5, 1.6, 3, 1.5, 250, "a plate=350", "aka:špagete bolonjez|pasta bolognese"),
	cf("caesar-salad", "Chicken Caesar salad", "meals", "g", 130, 9, 5, 8.5, 1.5, 1.5, 2, 300, "a bowl=300", "aka:salad|cezar salata"),
	cf("burger", "Hamburger", "meals", "g", 250, 12.5, 23, 12, 1.5, 5, 4.5, 450, "1 burger=220", "aka:cheeseburger|big mac"),
	cf("doner", "Döner kebab", "meals", "g", 215, 12, 20, 10, 1.6, 2, 3.5, 500, "1 döner=350", "aka:kebab|doner|gyros|shawarma"),
	cf("sandwich", "Ham and cheese sandwich", "meals", "g", 250, 13, 26, 10, 2, 3, 4.5, 800, "1 sandwich=150", "aka:sendvič|sendvic|toast sandwich"),
	cf("sushi", "Sushi roll", "meals", "g", 140, 4.5, 26, 2, 1, 3, 0.4, 330, "1 piece=30", "8 pieces=240", "aka:maki|california roll"),
	cf("crepe", "Pancake (crêpe), plain", "meals", "g", 227, 6, 28, 10, 0.8, 5, 3, 200, "1 crêpe=60", "aka:palačinka|palacinke|pancake"),
}

var catalogIdx = func() map[string]*CatalogFood {
	m := map[string]*CatalogFood{}
	for i := range Catalog {
		m[Catalog[i].Key] = &Catalog[i]
	}
	return m
}()

// CatalogByKey finds a built-in food.
func CatalogByKey(key string) (*CatalogFood, bool) {
	c, ok := catalogIdx[key]
	return c, ok
}

// Food is the built-in food as a person's food (not yet saved).
func (c *CatalogFood) Food() Food {
	return Food{Name: c.Name, Builtin: c.Key, Category: c.Category, Unit: c.Unit, Nutrients: c.Nutrients, Servings: append([]Serving(nil), c.Servings...), Source: "builtin"}
}

// NormName folds a name for matching: lower case, no accents (č → c,
// đ → d), only letters and digits, single spaces.
func NormName(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, strings.ToLower(s))
	if err != nil {
		out = strings.ToLower(s)
	}
	out = strings.NewReplacer("đ", "d", "ß", "ss", "æ", "ae", "ø", "o").Replace(out)
	var b strings.Builder
	space := false
	for _, r := range out {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		} else {
			space = true
		}
	}
	return b.String()
}

// matchScore says how well a query fits a name (0 = not at all): the whole
// name, its start, a word's start, anywhere.
func matchScore(q, name string) int {
	switch {
	case q == "":
		return 1
	case name == q:
		return 100
	case strings.HasPrefix(name, q):
		return 80
	case strings.Contains(" "+name, " "+q):
		return 60
	case strings.Contains(name, q):
		return 40
	}
	words := strings.Fields(q)
	if len(words) > 1 {
		for _, w := range words {
			if !strings.Contains(name, w) {
				return 0
			}
		}
		return 30
	}
	return 0
}

// SearchCatalog finds built-in foods by name or alias, best first.
func SearchCatalog(q string, limit int) []*CatalogFood {
	q = NormName(q)
	type hit struct {
		c     *CatalogFood
		score int
	}
	var hits []hit
	for i := range Catalog {
		c := &Catalog[i]
		best := matchScore(q, NormName(c.Name))
		for _, a := range c.Aliases {
			if sc := matchScore(q, NormName(a)) - 5; sc > best {
				best = sc
			}
		}
		if best > 0 {
			hits = append(hits, hit{c, best})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	out := []*CatalogFood{}
	for _, h := range hits {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, h.c)
	}
	return out
}

// CatalogMatch is the built-in food a name means, when it clearly means one.
func CatalogMatch(name string) (*CatalogFood, bool) {
	n := NormName(name)
	for i := range Catalog {
		c := &Catalog[i]
		if NormName(c.Name) == n || c.Key == n {
			return c, true
		}
		for _, a := range c.Aliases {
			if NormName(a) == n {
				return c, true
			}
		}
	}
	return nil, false
}
