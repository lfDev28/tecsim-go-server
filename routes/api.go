package routes

// Import the necessary packages

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
)

type AssetDetail struct {
	id         int
	productId  int
	categoryId int
}

func PassAllAssets(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get the locationId and userId from the request body
		var Request struct {
			LocationId int    `json:"locationId"`
			UserId     string `json:"userId"`
		}

		if err := c.BodyParser(&Request); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"success": false,
				"message": "Error parsing request body",
			})
		}

		initialLocation := []int{Request.LocationId}
		// Fetch all the locations under and including the provided location
		allLocations, err := fetchChildLocations(initialLocation, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": err.Error(),
			})
		}

		// Convert the locations slice to a map for easier batch processing
		locationMap := make(map[int]struct{})
		for _, loc := range allLocations {
			locationMap[loc] = struct{}{}
		}

		// Fetch assets for all these locations
		assetsMap, err := fetchAssetsForLocations(allLocations, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": err.Error(),
			})
		}

		// Collate all assets from the map into a single slice
		var allAssets []AssetDetail
		for _, assets := range assetsMap {
			allAssets = append(allAssets, assets...)
		}

		categoryIds := make(map[int]struct{})
		for _, asset := range allAssets {
			categoryIds[asset.categoryId] = struct{}{}
		}

		checkGroupTemplatesMap, err := fetchCheckGroupTemplates(categoryIds, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Error fetching check group templates",
			})
		}

		checkItemTemplatesMap, err := fetchCheckItemsTemplates(checkGroupTemplatesMap, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": err.Error(),
			})
		}

		err = processAndInsertAssets(allAssets, checkGroupTemplatesMap, checkItemTemplatesMap, Request.UserId, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": err.Error(),
			})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Successfully passed all assets",
		})
	}
}

func fetchAssetsForLocations(locationIds []int, db *sql.DB) (map[int][]AssetDetail, error) {
	// Convert the slice of location IDs into a comma-separated string.
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(locationIds)), ",")

	const baseQuery = `
        SELECT a.id AS assetId, p.id AS productId, p.categoryId
        FROM Assets AS a
        JOIN Product AS p ON a.productId = p.id
        WHERE a.locationId IN (%s)
    `

	query := fmt.Sprintf(baseQuery, placeholders)

	interfaceArgs := make([]interface{}, len(locationIds))
	for i, v := range locationIds {
		interfaceArgs[i] = v
	}

	rows, err := db.Query(query, interfaceArgs...)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assetsMap := make(map[int][]AssetDetail)
	for rows.Next() {
		var asset AssetDetail
		var locationId int
		err = rows.Scan(&asset.id, &asset.productId, &asset.categoryId)
		if err != nil {
			return nil, err
		}
		assetsMap[locationId] = append(assetsMap[locationId], asset)
	}

	// Check for errors from iterating over rows.
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Print the assets map

	return assetsMap, nil
}
func interfaceSlice(slice []int) []interface{} {
	s := make([]interface{}, len(slice))
	for i, v := range slice {
		s[i] = v
	}
	return s
}

func fetchChildLocations(locationIds []int, db *sql.DB) ([]int, error) {
	fmt.Println("Running child locations func")
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(locationIds)), ",")
	query := `SELECT id FROM Locations WHERE parentId IN (` + placeholders + `)`

	rows, err := db.Query(query, interfaceSlice(locationIds)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var childIds []int
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		childIds = append(childIds, id)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return childIds, nil
}

type CheckGroupTemplate struct {
	id         int
	name       string
	categoryId int
}

func fetchCheckGroupTemplates(categoryIds map[int]struct{}, db *sql.DB) (map[int][]CheckGroupTemplate, error) {
	checkGroupTemplatesMap := make(map[int][]CheckGroupTemplate)

	for categoryId := range categoryIds {
		query := `SELECT id, name, categoryId FROM CheckGroupTemplates WHERE categoryId = ?`

		rows, err := db.Query(query, categoryId)
		if err != nil {
			return nil, err
		}

		var templates []CheckGroupTemplate
		for rows.Next() {
			var template CheckGroupTemplate
			err = rows.Scan(&template.id, &template.name, &template.categoryId)
			if err != nil {
				rows.Close()
				return nil, err
			}
			templates = append(templates, template)

		}
		checkGroupTemplatesMap[categoryId] = templates

	}

	return checkGroupTemplatesMap, nil
}

type CheckItemTemplate struct {
	id           int
	name         string
	checkGroupId int
}

func fetchCheckItemsTemplates(checkGroupTemplatesMap map[int][]CheckGroupTemplate, db *sql.DB) (map[int][]CheckItemTemplate, error) {
	checkItemsTemplatesMap := make(map[int][]CheckItemTemplate)
	for _, templates := range checkGroupTemplatesMap {
		for _, template := range templates {
			query := `SELECT id, name, checkGroupId FROM CheckItemsTemplate WHERE checkGroupId = ?`
			rows, err := db.Query(query, template.id)

			if err != nil {
				return nil, err
			}

			var itemsTemplates []CheckItemTemplate
			for rows.Next() {
				var itemTemplate CheckItemTemplate
				err = rows.Scan(&itemTemplate.id, &itemTemplate.name, &itemTemplate.checkGroupId)
				if err != nil {
					rows.Close()
					return nil, err
				}
				itemsTemplates = append(itemsTemplates, itemTemplate)

			}
			checkItemsTemplatesMap[template.id] = itemsTemplates
			rows.Close()

		}
	}
	return checkItemsTemplatesMap, nil

}

const batchSize = 10

func processAndInsertAssets(assets []AssetDetail, checkGroupTemplatesMap map[int][]CheckGroupTemplate, checkItemTemplatesMap map[int][]CheckItemTemplate, userId string, db *sql.DB) error {

	// Split assets into batches
	for i := 0; i < len(assets); i += batchSize {
		end := i + batchSize
		if end > len(assets) {
			end = len(assets)
		}

		batch := assets[i:end]

		err := processBatch(batch, checkGroupTemplatesMap, checkItemTemplatesMap, userId, db)
		if err != nil {
			return err
		}
	}

	return nil
}

func processBatch(assets []AssetDetail, checkGroupTemplatesMap map[int][]CheckGroupTemplate, checkItemTemplatesMap map[int][]CheckItemTemplate, userId string, db *sql.DB) error {
	tx, err := db.Begin()

	fmt.Println("Running the processing of assets")

	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	for _, asset := range assets {
		fmt.Println("Processing asset: ", asset.id)

		assetCheckQuery := `INSERT INTO AssetCheck (assetId, owner, updatedAt, status) VALUES (?, ?, ?, ?)`
		result, err := tx.Exec(assetCheckQuery, asset.id, userId, time.Now(), "pass")
		if err != nil {
			return err
		}

		assetCheckId, err := result.LastInsertId()
		if err != nil {
			return err
		}

		for _, checkGroupTemplate := range checkGroupTemplatesMap[asset.categoryId] {
			checkGroupQuery := `INSERT INTO CheckGroup (name, assetCheckId, status) VALUES (?, ?, ?)`
			result, err := tx.Exec(checkGroupQuery, checkGroupTemplate.name, assetCheckId, "pass")
			if err != nil {
				return err
			}

			checkGroupId, err := result.LastInsertId()
			if err != nil {
				return err
			}

			for _, checkItemTemplate := range checkItemTemplatesMap[checkGroupTemplate.id] {
				checkItemQuery := `INSERT INTO CheckItems (name, checkGroupId, status, updatedAt) VALUES (?, ?, ?, ?)`
				_, err := tx.Exec(checkItemQuery, checkItemTemplate.name, checkGroupId, "pass", time.Now())
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
