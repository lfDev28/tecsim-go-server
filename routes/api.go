package routes

// Import the necessary packages

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
)

type AssetDetail struct {
	id         int
	productId  int
	categoryId int
}

// Create the fiber handler
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

		// TODO: Call all the helper functions to create all of the asset checks

		assets, err := fetchAssetsForLocation(Request.LocationId, db)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": err.Error(),
			})
		}

		categoryIds := make(map[int]struct{})
		for _, asset := range assets {
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

		err = processAndInsertAssets(assets, checkGroupTemplatesMap, checkItemTemplatesMap, Request.UserId, db)

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

// Implement first handeler function to get all of the assets by the locationId
// This function is incomplete as it needs to traverse all childLocations as well
// TODO!!!
func fetchAssetsForLocation(locationId int, db *sql.DB) ([]AssetDetail, error) {
	const query = `
        SELECT a.id AS assetId, p.id AS productId, p.categoryId
        FROM Assets AS a
        JOIN Product AS p ON a.productId = p.id
        WHERE a.locationId = ?
    `

	rows, err := db.Query(query, locationId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []AssetDetail
	for rows.Next() {
		var asset AssetDetail
		err = rows.Scan(&asset.id, &asset.productId, &asset.categoryId)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}

	// Check for errors from iterating over rows.
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return assets, nil
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

func processAndInsertAssets(assets []AssetDetail, checkGroupTemplatesMap map[int][]CheckGroupTemplate, checkItemTemplatesMap map[int][]CheckItemTemplate, userId string, db *sql.DB) error {
	tx, err := db.Begin()

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

	// Create all of the asset checks with the user Id and the asset Id

	for _, asset := range assets {
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
