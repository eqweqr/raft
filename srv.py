import socket 
import cv2
import pickle
import struct


id = 1
payload_size = struct.calcsize("Q")
id_size = struct.calcsize("h")

with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
    s.bind(('localhost', 8080))
    data = b""
    s.listen()
    while True:
        client_socket, addr = s.accept()
        with client_socket:
            while True:
                while len(data)<payload_size:
                    packet = client_socket.recv(4*1024)
                    if not packet: break
                    data += packet
                packed_msg_size = data[:payload_size]
                id = data[payload_size:payload_size+id_size]
                data = data[payload_size+id_size:]
                print(struct.unpack("h", id)[0])
                msg_size = struct.unpack("Q", packed_msg_size)[0]
                while len(data)<msg_size:
                    data += client_socket.recv(4*1024)
                frame_data = data[:msg_size]
                data =data[msg_size:]
                frame = pickle.loads(frame_data)
                cv2.imshow('img', frame)
                key = cv2.waitKey(10)
                if key == 13:
                    break