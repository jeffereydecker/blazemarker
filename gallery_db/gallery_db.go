package gallery_db

import (
	"bufio"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/jeffereydecker/blazemarker/blaze_log"
	"github.com/disintegration/imaging"
)

var logger = blaze_log.GetLogger()

// albumCoverSuffix
// Name    post    X     Y     min required
// SQUARE    -sq    120    120    Yes
// THUMB    -th    144    144    Yes
// XXSMALL    -2s    240    240
// XSMALL    -xs    432    324
// SMALL    -sm    576    432
// MEDIUM     -me    792    594    Yes
// LARGE    -la    1008    756
// XLARGE    -xl    1224    918
// XXLARGE    -xx    1656    1242

var sitePhotoFormatsWidth = map[string]int{
	"-sq": 120,
	"-th": 144,
	"-2s": 240,
	"-xs": 432,
	"-sm": 576,
	"-me": 792,
	"-la": 1008,
	"-xl": 1224,
	"-xx": 1656,
}

var sitePhotoFormatsHeight = map[string]int{
	"-sq": 120,
	"-th": 144,
	"-2s": 240,
	"-xs": 324,
	"-sm": 432,
	"-me": 594,
	"-la": 756,
	"-xl": 918,
	"-xx": 1242,
}

type Album struct {
	Index          int      `json:"index"`
	Name           string   `json:"name"`
	Path           string   `json:"path"`
	SitePhotos     []*Photo `json:"site_photos"`
	OriginalPhotos []*Photo `json:"original_photos"`
}

type Photo struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Index int    `json:"index"`
	Name  string `json:"name"`
	Path  string `json:"path"`
}

var jpg_expression = `\.(?i)jpg`
var jpg_re = regexp.MustCompile(jpg_expression)

func findFirstJPG(albumPath string, album os.DirEntry) (string, os.FileInfo) {
	logger.Debug("findFirstJPG",
		"albumPath", albumPath,
		"album.Name()", album.Name())

	if album.IsDir() {
		albumFullPath := albumPath + album.Name() + `/`
		photos, err := os.ReadDir(albumFullPath)
		if err != nil {
			logger.Error(err.Error())
			return "", nil
		}

		// For each album file/picture
		for _, photo := range photos {
			photoName := photo.Name()
			if !photo.IsDir() && jpg_re.FindStringIndex(photo.Name()) != nil {
				photoFullPath := albumFullPath + photoName
				fi, err := os.Stat(photoFullPath)
				if err != nil {
					logger.Error(err.Error())
					return "", nil
				}
				// get the size
				if fi.Size() > 0 {
					return photoFullPath, fi
				}
			}
		}
	}
	return "", nil

}

func findOrAddSitePhotoDir(album string) (string, os.FileInfo) {
	logger.Debug("findOrAddSitePhotoDir",
		"album", album)

	sitePhotoPath := album + `/.site_photos`
	fi, err := os.Stat(sitePhotoPath)

	if err != nil {
		// create directory and post check after creating
		err = os.Mkdir(sitePhotoPath, 0755)
		if err != nil {
			logger.Error(err.Error())
			return "", nil
		}

		fi, err = os.Stat(sitePhotoPath)
		if err != nil {
			logger.Error(err.Error())
			return "", nil
		}
	}

	if fi.IsDir() {
		return sitePhotoPath, fi
	}

	return "", nil
}

func findSitePhoto(albumPath string, album os.FileInfo, sourcePhotoName *string, photoSize string, photoType string) (string, os.FileInfo) {
	logger.Debug("findSitePhoto", "albumPath", albumPath,
		"album.Name()", album.Name(),
		"sourcePhotoName", sourcePhotoName,
		"photoSize", photoSize,
		"photoType", photoType)

	if album.IsDir() {
		photos, err := os.ReadDir(albumPath)
		if err != nil {
			logger.Error(err.Error())
			return "", nil
		}

		expression := ""
		photoPrefix := ""
		photoExt := `\.(?i)jpg`

		if sourcePhotoName != nil {
			photoPrefix = strings.TrimSuffix(*sourcePhotoName, filepath.Ext(*sourcePhotoName))
		}

		expression = photoPrefix + photoType + photoSize + photoExt
		re := regexp.MustCompile(expression)

		for _, photo := range photos {
			if !photo.IsDir() && re.FindStringIndex(photo.Name()) != nil {
				sitePhotoFullPath := albumPath + `/` + photo.Name()
				fi, err := os.Stat(sitePhotoFullPath)
				if err != nil {
					logger.Error(err.Error())
					return "", nil
				}
				if fi.Size() > 0 {
					return sitePhotoFullPath, fi
				}
			}
		}
	}
	return "", nil
}

func createSitePhoto(imageSourcePath string, imageName string, imageDestPath string, imageDestDir os.FileInfo, photoType string, photoSize string) (string, os.FileInfo) {

	logger.Debug("createSitePhoto",
		"imageSourcePath", imageSourcePath,
		"imageName", imageName,
		"imageDestPath", imageDestPath,
		"imageDestDir", imageDestDir,
		"photoType", photoType,
		"photoSize", photoSize)

	// maximize CPU usage for maximum performance
	runtime.GOMAXPROCS(runtime.NumCPU())

	img, err := imaging.Open(imageSourcePath)
	if err != nil {
		logger.Error(err.Error())
		return "", nil
	}

	inputFile, err := os.Open(imageSourcePath)
	if err != nil {
		logger.Error(err.Error())
		return "", nil
	}

	defer inputFile.Close()

	reader := bufio.NewReader(inputFile)
	config, format, err := image.DecodeConfig(reader)
	if err != nil {
		logger.Error(err.Error())
		return "", nil
	}

	logger.Debug("image details",
		"imageSourcePath", imageSourcePath,
		"config.Width", config.Width,
		"config.Height", config.Height,
		"format", format)

	landscape := config.Width > config.Height

	// resize image from 1000 to 500 while preserving the aspect ration
	// Supported resize filters: NearestNeighbor, Box, Linear, Hermite, MitchellNetravali,
	// CatmullRom, BSpline, Gaussian, Lanczos, Hann, Hamming, Blackman, Bartlett, Welch, Cosine.

	//dstimg := imaging.Resize(img, sitePhotoFormatsWidth[albumCoverSize], sitePhotoFormatsHeight[albumCoverSize], imaging.Lanczos)

	width := sitePhotoFormatsWidth[photoSize]
	height := sitePhotoFormatsHeight[photoSize]

	if !landscape {
		width = sitePhotoFormatsHeight[photoSize]
		height = sitePhotoFormatsWidth[photoSize]

	}

	dstimg := imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos)

	// save resized image
	prefixImageName := strings.TrimSuffix(imageName, filepath.Ext(imageName))
	newImageName := prefixImageName + photoType + photoSize + ".jpg"
	destImageFullPath := imageDestPath + `/` + newImageName
	err = imaging.Save(dstimg, destImageFullPath)

	if err != nil {
		logger.Error(err.Error())
		return "", nil
	}

	newImage, err := os.Stat(destImageFullPath)
	if err != nil {
		logger.Error(err.Error())
		return "", nil
	}

	if newImage.Size() > 0 {
		return destImageFullPath, newImage
	}

	return "", nil
}

func findOrAddAlbumCover(albumPath string, album os.DirEntry, photoSize string) (string, os.FileInfo) {
	logger.Debug("findOrAddAlbumCover", "albumPath", albumPath, "album.Name()", album.Name(), "photoSize", photoSize)

	if sitePhotoPath, sitePhotoDir := findOrAddSitePhotoDir(albumPath + album.Name()); len(sitePhotoPath) > 0 && sitePhotoDir != nil {
		if albumCoverPath, albumCover := findSitePhoto(sitePhotoPath, sitePhotoDir, nil, photoSize, "-ac"); len(albumCoverPath) > 0 && albumCover != nil {
			return albumCoverPath, albumCover
		}
		if photoPath, photo := findFirstJPG(albumPath, album); len(photoPath) > 0 && photo != nil {
			albumCoverPath, albumCover := createSitePhoto(photoPath, photo.Name(), sitePhotoPath, sitePhotoDir, "-ac", photoSize)
			return albumCoverPath, albumCover
		}
	}

	return "", nil
}

func findOrAddSitePhoto(photoPath string, photoName string, photoSize string) *Photo {
	//TODO: Replace photo os.FileInfo with pagePhoto *Photo
	var pagePhoto *Photo = nil

	logger.Debug("findOrAddSitePhoto", "photoPath", photoPath, "photoName", photoName)

	if sitePhotoDirPath, sitePhotoDir := findOrAddSitePhotoDir(photoPath); len(sitePhotoDirPath) > 0 && sitePhotoDir != nil {
		if foundSitePhotoPath, foundSitePhoto := findSitePhoto(sitePhotoDirPath, sitePhotoDir, &photoName, photoSize, "-gp"); len(foundSitePhotoPath) > 0 && foundSitePhoto != nil {

			pagePhoto = new(Photo)
			pagePhoto.Name = photoName
			pagePhoto.Path = foundSitePhotoPath

		} else {
			if newSitePhotoPath, newSitePhoto := createSitePhoto(photoPath+photoName, photoName, sitePhotoDirPath, sitePhotoDir, "-gp", photoSize); len(newSitePhotoPath) > 0 && newSitePhoto != nil {
				pagePhoto = new(Photo)
				pagePhoto.Name = photoName
				pagePhoto.Path = newSitePhotoPath
			}
		}

	}

	return pagePhoto
}

func GetAllAlbums() []*Album {
	photoPath := "../photos/galleries/"

	files, err := os.ReadDir(photoPath)
	if err != nil {
		logger.Error(err.Error())
		return nil
	}

	var albumIndex = 0

	logger.Debug("GetAllAlbums()", "albumIndex", albumIndex)
	albums := make([]*Album, 0)
	for _, fileAlbum := range files {
		if fileAlbum.IsDir() {
			if albumCoverPath, albumCover := findOrAddAlbumCover(photoPath, fileAlbum, "-xs"); len(albumCoverPath) > 0 && albumCover != nil {
				//TODO: wider use of album
				album := new(Album)
				album.Index = albumIndex
				albumIndex = albumIndex + 1
				album.Name = fileAlbum.Name()
				album.Path = albumCoverPath
				albums = append(albums, album)
			}
		}
	}

	return albums
}

func GetAlbumPhotos(albumName string) (sitePhotos []*Photo, originalPhotos []*Photo) {

	path := "../photos/galleries/" + albumName + "/"

	logger.Debug("GetAlbumPhoto()", "albumName", albumName, "path", path)

	photos, err := os.ReadDir(path)
	if err != nil {
		logger.Error(err.Error())
		return nil, nil
	}

	sitePhotos = make([]*Photo, 0)
	originalPhotos = make([]*Photo, 0)

	var photoIndex = 0

	for _, photo := range photos {
		if !photo.IsDir() && jpg_re.FindStringIndex(photo.Name()) != nil {
			if pagePhoto := findOrAddSitePhoto(path, photo.Name(), "-xl"); pagePhoto != nil {
				pagePhoto.Index = photoIndex
				sitePhotos = append(sitePhotos, pagePhoto)
				pageOriginalPhoto := new(Photo)
				pageOriginalPhoto.Name = photo.Name()
				pageOriginalPhoto.Path = path + photo.Name()
				pageOriginalPhoto.Index = photoIndex
				originalPhotos = append(originalPhotos, pageOriginalPhoto)
				photoIndex = photoIndex + 1
			}
		}
	}
	return sitePhotos, originalPhotos
}
