package main

func createSitePhoto_ch(imageSourcePath string, imageName string, imageDestPath string, imageDestDir os.FileInfo, photoType string, photoSize string, wg *sync.WaitGroup) (string, os.FileInfo) {
	wg.Add(1)
	defer wg.Done()

	// maximize CPU usage for maximum performance
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Printf("1a: createSitePhoto(), imageSourcePath=%s, imageSourceName=%s, imageDestPath=%s, imageDestDir.Name()=%s, photoType=%s, photoSize=%s \n", imageSourcePath, imageName, imageDestPath, imageDestDir.Name(), photoType, photoSize)
	
	img, err := imaging.Open(imageSourcePath)

	fmt.Println("1b: createSitePhoto()")
	
	if err != nil {
		fmt.Println("1c: createSitePhoto()")
		fmt.Println(err)
		os.Exit(1)
	}
	

	fmt.Println("2: createSitePhoto()")
	inputFile, _ := os.Open(imageSourcePath)
	defer inputFile.Close()
	
	reader := bufio.NewReader(inputFile)
	config, _, _ := image.DecodeConfig(reader)


	fmt.Println("3: createSitePhoto()")
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

	fmt.Println("4: createSitePhoto()")
	dstimg := imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos)

	fmt.Println("5: createSitePhoto(), imaging.Fill")

	// save resized image
	prefixImageName := strings.TrimSuffix(imageName, filepath.Ext(imageName))
	newImageName := prefixImageName + photoType + photoSize + ".jpg"
	fmt.Println("6: createSitePhoto(), newImageName=" + newImageName)
	destImageFullPath := imageDestPath + `/` + newImageName
	fmt.Println("7: createSitePhoto(), imaging.Save, destImageFullPath=", destImageFullPath)
	err = imaging.Save(dstimg, destImageFullPath)
	fmt.Println("8: createSitePhoto()")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// everything ok
	fmt.Println("9: createSitePhoto, Site Image resized and saved")
	newImage, err := os.Stat(destImageFullPath)
	if err != nil {
		log.Fatal(err)
	}
	if newImage.Size() > 0 {
		return destImageFullPath, newImage
	}

	fmt.Println("10: createSitePhoto()")
	return "", nil
}



func findOrAddSitePhoto_ch(photoPath string, photo os.FileInfo, photoSize string, worker_ch chan int, photo_ch chan *Photo, wg *sync.WaitGroup, mtx *sync.Mutex) () {
	//TODO: Ordered channel? Create or read
	defer wg.Done()

	index := <- worker_ch
	fmt.Printf("1a: findOrAddSitePhoto_ch - Another worker %d \n", index)
	
	if sitePhotoPath, sitePhotoDir := findOrAddSitePhotoDir(photoPath); len(sitePhotoPath) > 0 && sitePhotoDir != nil {
		fmt.Printf("1b: findOrAddSitePhoto_ch(), sitePhotoPath=%s, sitePhotoDir.Name()=%s \n", sitePhotoPath, sitePhotoDir.Name())
		if foundSitePhotoPath, _ := findSitePhoto(sitePhotoPath, sitePhotoDir, photo, photoSize, "-gp"); len(foundSitePhotoPath) > 0  {
			fmt.Printf("2: findOrAddSitePhoto_ch")
			pagePhoto:=new(Photo)
			pagePhoto.Index = index-1
			pagePhoto.Name = photo.Name()
			pagePhoto.Path = foundSitePhotoPath
			photo_ch <- pagePhoto
		} else {
			mtx.Lock()
			if newSitePhotoPath, _ := createSitePhoto_ch(photoPath+photo.Name(), photo, sitePhotoPath, sitePhotoDir, "-gp", photoSize, wg); len(newSitePhotoPath) > 0  {
				fmt.Printf("3: findOrAddSitePhoto_ch")
				pagePhoto:=new(Photo)
				pagePhoto.Index = index-1
				pagePhoto.Name = photo.Name()
				pagePhoto.Path = newSitePhotoPath
				photo_ch <- pagePhoto
			}
			mtx.Unlock()
		}
	}

	fmt.Printf("4: findOrAddSitePhoto_ch")
}


func servAlbum_parallel(w http.ResponseWriter, r *http.Request) {

	query := r.URL.Query()
	albumName := query.Get("name")
	
	if len(albumName) == 0 {
		fmt.Println("servAlbum(), HTTP Request filters not present")
		return
	}
	
	if !basicAuth(w, r) {
		return
	}
	
	fmt.Println("/album, servAlbum(), r.URL.Path:" + r.URL.Path)

	pageData := new(Album)
	pageData.Name = albumName
	pageData.Photos = make([]*Photo, 0)



	

	albumPath := "photos/galleries/" + albumName + "/"
	pageData.Path=albumPath

	photos, err := ioutil.ReadDir(albumPath)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("pagePhoto.Name = %s, pageData.Path = %s\n", pageData.Name, pageData.Path)



	photo_ch := make (chan *Photo)
	worker_ch := make (chan int, 1)

	wg := new (sync.WaitGroup)
	mtx := new (sync.Mutex)
	fmt.Printf("1: findOrAddSitePhoto_ch servAlbumTmp \n")
	for index, photo := range photos {
		fmt.Printf("2: findOrAddSitePhoto_chservAlbumTmpl, index=%d  \n", index)
		if !photo.IsDir() && jpg_re.FindStringIndex(photo.Name()) != nil {
			worker_ch <- index
			wg.Add(1)
			fmt.Printf("3: findOrAddSitePhoto_chservAlbumTmpl, index=%d  \n", index)
			go  findOrAddSitePhoto_ch(albumPath, photo, "-xl", worker_ch, photo_ch, wg, mtx)
			fmt.Printf("4: findOrAddSitePhoto_chservAlbumTmpl, index=%d  \n", index)
		}
	}

	go func() {
		for pagePhoto := range photo_ch {
			fmt.Printf("pagePhoto.Index = %d, pagePhoto.Name = %s, pagePhoto.Path = %s \n", pagePhoto.Index, pagePhoto.Name, pagePhoto.Path)
			pageData.Photos = append(pageData.Photos, pagePhoto)
			
		}
	}()



	fmt.Printf("5: findOrAddSitePhoto_chservAlbumTmpl  \n")
	wg.Wait()
	fmt.Printf("6: findOrAddSitePhoto_chservAlbumTmpl  \n")
	close(photo_ch)
	fmt.Printf("7: findOrAddSitePhoto_chservAlbumTmpl  \n")

	
	t, _ := template.ParseFiles("templates/base.html", "templates/album.html")
	err = t.Execute(w, pageData)

	if err != nil {
		log.Fatal(err)
	}
}

