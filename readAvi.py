import cv2
from keras_facenet import FaceNet
import collections
import pathlib
import os
from PIL import Image
import numpy as np
from time import time
import json
from numpy import linalg as LA
from utils import highlightFace, cutFace


global globalDB
curPath = pathlib.Path(r'C:\Users\Leonard\Desktop\kursoc')
toSave = pathlib.Path(r'C:\Users\Leonard\Desktop\kursoc\tt\1.jpg')
f = FaceNet()
faceProto=curPath / "model\opencv_face_detector.pbtxt"
faceModel=curPath / "model\opencv_face_detector_uint8.pb"
recImg = pathlib.Path(r'C:\Users\Leonard\Desktop\kursoc\facesToRecognize')
embeddingDir = pathlib.Path(r'C:\Users\Leonard\Desktop\kursoc\embForFR\embedding.json')
faceNet=cv2.dnn.readNet(faceModel,faceProto)


def encodeImgFromPath(path):
    # читает изображение и выдает вектор
    img = cv2.imread(path)
    emb = f.embeddings([img])
    return emb


def writeAllDataset(dir=recImg, embDir = embeddingDir):
    # полностью перезаписывает embedding для всей директории
    d = []
    for file in os.listdir(recImg):
        tmp = {}
        tmp['name']=file[:-4]
        print(tmp['name'])
        tmp['encode']=encodeImgFromPath(dir/file).tolist()
        d.append(tmp)
    with open(embDir, 'w') as f:
        json.dump(d, f)


def loadEmbeddings(path=embeddingDir):
    # load from embedding to cur memory
    f = open(path, 'r')
    dataSet=json.load(f)
    curDataset = [] 
    for row in dataSet:
        entity = {}
        entity['name'] = row['name']
        entity['encode'] = np.asarray(row['encode'])
        curDataset.append(entity)
    return curDataset


def readFromVideo(path):
    cap = cv2.VideoCapture(path)
    cap.set(cv2.CAP_PROP_POS_FRAMES, 50)
    print(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    if (cap.isOpened()== False):
        print("Error opening video file")

    i = 0
    while(cap.isOpened()):
        ret, frame = cap.read()
        if ret == True:
            resultImg,rest=cutFace(faceNet,frame)
            emb = f.embeddings([rest])
            for row in globalDB:
                if (LA.norm(row['encode']-emb))<0.6:
                    print(row['name'])
                    break
            cv2.imshow("Face detection", resultImg) 

            if cv2.waitKey(25) & 0xFF == ord('q'):
                break
        else:
            break
    cap.release()
    cv2.destroyAllWindows()


if __name__ == "__main__":
    globalDB = loadEmbeddings()
    readFromVideo(curPath/'./to_validate.avi')

